package cmd

import (
	"fmt"
	"io"
	"os"
	"path"

	systemd "github.gatech.edu/faasedge/fecore/pkg/systemd"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install fecore",
	RunE:  runInstall,
}

const workingDirectoryPermission = 0644

const fecorewd = "/var/lib/fecore"

const fecoreProviderWd = "/var/lib/fecore-provider"

func runInstall(_ *cobra.Command, _ []string) error {
  fmt.Println("Installation from within fecore is currently not supported. Please follow manual installation steps instead.");
  return nil

	if err := ensureWorkingDir(path.Join(fecorewd, "secrets")); err != nil {
		return err
	}

	if err := ensureWorkingDir(fecoreProviderWd); err != nil {
		return err
	}

	if basicAuthErr := makeBasicAuthFiles(path.Join(fecorewd, "secrets")); basicAuthErr != nil {
		return errors.Wrap(basicAuthErr, "cannot create basic-auth-* files")
	}

	if err := cp("docker-compose.yaml", fecorewd); err != nil {
		return err
	}

	if err := cp("prometheus.yml", fecorewd); err != nil {
		return err
	}

	if err := cp("resolv.conf", fecorewd); err != nil {
		return err
	}

	err := binExists("/usr/local/bin/", "fecore")
	if err != nil {
		return err
	}

	err = systemd.InstallUnit("fecore-provider", map[string]string{
		"Cwd":             fecoreProviderWd,
		"SecretMountPath": path.Join(fecorewd, "secrets")})

	if err != nil {
		return err
	}

	err = systemd.InstallUnit("fecore", map[string]string{"Cwd": fecorewd})
	if err != nil {
		return err
	}

	err = systemd.DaemonReload()
	if err != nil {
		return err
	}

	err = systemd.Enable("fecore-provider")
	if err != nil {
		return err
	}

	err = systemd.Enable("fecore")
	if err != nil {
		return err
	}

	err = systemd.Start("fecore-provider")
	if err != nil {
		return err
	}

	err = systemd.Start("fecore")
	if err != nil {
		return err
	}

	fmt.Println(`Check status with:
  sudo journalctl -u fecore --lines 100 -f

Login with:
  sudo cat /var/lib/fecore/secrets/basic-auth-password | faas-cli login -s`)

	return nil
}

func binExists(folder, name string) error {
	findPath := path.Join(folder, name)
	if _, err := os.Stat(findPath); err != nil {
		return fmt.Errorf("unable to stat %s, install this binary before continuing", findPath)
	}
	return nil
}
func ensureSecretsDir(folder string) error {
	if _, err := os.Stat(folder); err != nil {
		err = os.MkdirAll(folder, secretDirPermission)
		if err != nil {
			return err
		}
	}

	return nil
}
func ensureWorkingDir(folder string) error {
	if _, err := os.Stat(folder); err != nil {
		err = os.MkdirAll(folder, workingDirectoryPermission)
		if err != nil {
			return err
		}
	}

	return nil
}

func cp(source, destFolder string) error {
	file, err := os.Open(source)
	if err != nil {
		return err

	}
	defer file.Close()

	out, err := os.Create(path.Join(destFolder, source))
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, file)

	return err
}
