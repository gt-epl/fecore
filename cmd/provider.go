package cmd

import (
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/jasonlvhit/gocron"
	bootstrap "github.com/openfaas/faas-provider"
	"github.com/openfaas/faas-provider/logs"
	"github.com/openfaas/faas-provider/types"
	"github.com/spf13/cobra"
	fecore "github.gatech.edu/faasedge/fecore/pkg"
	"github.gatech.edu/faasedge/fecore/pkg/cninetwork"
	fecorelogs "github.gatech.edu/faasedge/fecore/pkg/logs"
	"github.gatech.edu/faasedge/fecore/pkg/provider/config"
	"github.gatech.edu/faasedge/fecore/pkg/provider/handlers"
	"github.gatech.edu/faasedge/fecore/pkg/provider/proxy"
	"github.gatech.edu/faasedge/fecore/pkg/provider/storage"
	"github.gatech.edu/faasedge/fecore/pkg/timec"
	_ "modernc.org/sqlite"
)

const secretDirPermission = 0755
const FECORE_CONFIG_FILE = "/mnt/faasedge/feconfig.json"
const FECORE_DB_FILE = "/mnt/faasedge/fecore.db"

func makeProviderCmd() *cobra.Command {
	var command = &cobra.Command{
		Use:   "provider",
		Short: "Run the fecore-provider",
	}

	command.Flags().String("pull-policy", "Always", `Set to "Always" to force a pull of images upon deployment, or "IfNotPresent" to try to use a cached image.`)

	command.RunE = func(_ *cobra.Command, _ []string) error {

		pullPolicy, flagErr := command.Flags().GetString("pull-policy")
		if flagErr != nil {
			return flagErr
		}

		alwaysPull := false
		if pullPolicy == "Always" {
			alwaysPull = true
		}

		cfg, err := config.LoadConfig(FECORE_CONFIG_FILE)
		if err != nil {
			timec.LogEvent("provider", fmt.Sprintf("ERROR: Could not load/parse config file at %s: %s", FECORE_CONFIG_FILE, err), 1)
			timec.WriteEventLog()
			timec.WriteDurationLog()
			os.Exit(-1)
		} else {
			timec.LogEvent("provider", "Loaded config from feconfig.json", 2)
		}

		config, providerConfig, err := config.ReadFromEnv(types.OsEnv{})
		if err != nil {
			return err
		}

		log.Printf("fecore-provider starting...\tService Timeout: %s\n", config.WriteTimeout.String())
		printVersion()

		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		writeHostsErr := ioutil.WriteFile(path.Join(wd, "hosts"),
			[]byte(`127.0.0.1	localhost`), workingDirectoryPermission)

		if writeHostsErr != nil {
			return fmt.Errorf("cannot write hosts file: %s", writeHostsErr)
		}

		writeResolvErr := ioutil.WriteFile(path.Join(wd, "resolv.conf"),
			[]byte(`nameserver 8.8.8.8`), workingDirectoryPermission)

		if writeResolvErr != nil {
			return fmt.Errorf("cannot write resolv.conf file: %s", writeResolvErr)
		}

		cni, err := cninetwork.InitNetwork()
		if err != nil {
			return err
		}

		client, err := containerd.New(providerConfig.Sock)
		if err != nil {
			return err
		}

		defer client.Close()

		db, err := sql.Open("sqlite", FECORE_DB_FILE)
		if err != nil {
			return err
		}
		storageManager, err := storage.NewSQLiteStorageManager(db)
		if err != nil {
			return err
		}

		fs, err := handlers.InitFunctionStore(storageManager, cfg)
		fs.Client = client
		fs.CNI = &cni

		go func() {
			gocron.Every(uint64(cfg.ContainerCleanupInterval)).Second().Do(fs.CleanupDaemon, client, cni)
			<-gocron.Start()
		}()

		/* Drain stats channel in the background */
		go func() {
			fs.ProcessFunctionStats()
		}()

		/* Watch for shutdown signal so we can cleanup gracefully */
		go func() {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

			log.Printf("fecore: waiting for SIGTERM or SIGINT\n")
			<-sig

			log.Printf("Signal received. Shutting down server")
			// err := supervisor.Remove(services)
			// if err != nil {
			// 	fmt.Println(err)
			// }
			timec.WriteEventLog()
			timec.WriteDurationLog()
			if cfg.UseDatabase != 1 {
				os.Remove("/mnt/faasedge/sqlite.db")
			}
			os.Exit(0)
		}()

		fs.CreateWasmInterfaces(cfg.MaxWasmContainers)

		invokeResolver := handlers.NewInvokeResolver(client, cni, fs)

		baseUserSecretsPath := path.Join(wd, "secrets")
		if err := moveSecretsToDefaultNamespaceSecrets(
			baseUserSecretsPath,
			fecore.DefaultFunctionNamespace); err != nil {
			return err
		}

		bootstrapHandlers := types.FaaSHandlers{
			FunctionProxy:        proxy.NewHandlerFunc(*config, invokeResolver, fs),
			DeleteHandler:        handlers.MakeDeleteHandler(client, cni, fs),
			DeployHandler:        handlers.MakeDeployHandler(client, cni, baseUserSecretsPath, alwaysPull, fs),
			FunctionReader:       handlers.MakeReadHandler(client, fs),
			ReplicaReader:        handlers.MakeReplicaReaderHandler(client, fs),
			ReplicaUpdater:       handlers.MakeReplicaUpdateHandler(client, cni),
			UpdateHandler:        handlers.MakeUpdateHandler(client, cni, baseUserSecretsPath, alwaysPull, fs),
			HealthHandler:        func(w http.ResponseWriter, r *http.Request) {},
			InfoHandler:          handlers.MakeInfoHandler(Version, GitCommit),
			ListNamespaceHandler: handlers.MakeNamespacesLister(client),
			SecretHandler:        handlers.MakeSecretHandler(client.NamespaceService(), baseUserSecretsPath),
			LogHandler:           logs.NewLogHandlerFunc(fecorelogs.New(), config.ReadTimeout),
		}

		bootstrap.Router().HandleFunc("/metrics", handlers.MakeMetricsHandler(fs))
		bootstrap.Router().HandleFunc("/policy", handlers.MakePolicyHandler(fs))
		bootstrap.Router().HandleFunc("/ipam", handlers.MakeIPAMHandler(fs))

		log.Printf("Listening on TCP port: %d\n", *config.TCPPort)
		bootstrap.Serve(&bootstrapHandlers, config)
		return nil
	}

	return command
}

/*
* Mutiple namespace support was added after release 0.13.0
* Function will help users to migrate on multiple namespace support of fecore
 */
func moveSecretsToDefaultNamespaceSecrets(baseSecretPath string, defaultNamespace string) error {
	newSecretPath := path.Join(baseSecretPath, defaultNamespace)

	err := ensureSecretsDir(newSecretPath)
	if err != nil {
		return err
	}

	files, err := ioutil.ReadDir(baseSecretPath)
	if err != nil {
		return err
	}

	for _, f := range files {
		if !f.IsDir() {

			newPath := path.Join(newSecretPath, f.Name())

			// A non-nil error means the file wasn't found in the
			// destination path
			if _, err := os.Stat(newPath); err != nil {
				oldPath := path.Join(baseSecretPath, f.Name())

				if err := copyFile(oldPath, newPath); err != nil {
					return err
				}

				log.Printf("[Migration] Copied %s to %s", oldPath, newPath)
			}
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	inputFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s failed %w", src, err)
	}
	defer inputFile.Close()

	outputFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_APPEND, secretDirPermission)
	if err != nil {
		return fmt.Errorf("opening %s failed %w", dst, err)
	}
	defer outputFile.Close()

	// Changed from os.Rename due to issue in #201
	if _, err := io.Copy(outputFile, inputFile); err != nil {
		return fmt.Errorf("writing into %s failed %w", outputFile.Name(), err)
	}

	return nil
}
