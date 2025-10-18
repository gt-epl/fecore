package handlers

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/containerd/containerd"
	"github.gatech.edu/faasedge/fecore/pkg"
	fecore "github.gatech.edu/faasedge/fecore/pkg"
)

type Function struct {
	name            string
	namespace       string
	image           string
	imageFiles      []string
	pid             map[string]uint32
	replicas        int
	sandboxes       map[string]string
	policy          Policy
	activeReplicas  map[string]*Replica
	idleReplicas    IdleReplicas
	idleReplicasTs  map[string]time.Time
	expiredReplicas []*Replica
	IP              string // not used
	labels          map[string]string
	annotations     map[string]string
	secrets         []string
	secretsPath     string
	envVars         map[string]string
	envProcess      string
	memoryLimit     int64
	createdAt       time.Time // not used
	/* Mutexes */
	fnMu               sync.RWMutex //lock for entire Function struct
	activeReplicasLock sync.RWMutex //lock for activeReplicas map
	idleReplicasLock   sync.RWMutex //lock for idleReplicas struct
	idleReplicasTsMu   sync.RWMutex //lock for idleReplicas timestamps map
	expiredReplicasMu  sync.RWMutex
	policyMu           sync.RWMutex
}

type wasmIPInfo struct {
	netnsNum int
	IP       string
}

type Replica struct {
	fname       string    // name of parent Function
	ctrType     string    // native or wasm container
	uuid        string    // unique ID
	PID         uint32    // PID of container
	IP          string    // IP of container
	netNS       int       // network namespace num of container
	lastAccess  time.Time // last time used
	accessCount int       // how many times container was used
}

type IdleReplicas struct {
	fname      string     // name of parent Function
	containers []*Replica // list of idle containers for Function
	count      uint32     // number of idle replicas for Function
	next       *Replica   // next idle replica to use
	LRU        *Replica
	MRU        *Replica
}

// // ListFunctions returns a map of all functions with running tasks on namespace
// func ListFunctions(client *containerd.Client, namespace string) (map[string]*Function, error) {

// 	// Check if namespace exists, and it has the proper label
// 	valid, err := validNamespace(client.NamespaceService(), namespace)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if !valid {
// 		return nil, errors.New("namespace not valid")
// 	}

// 	/* TODO: Eventually we will probably want to return the list of Functions
// 	 * stored in the database. For now we'll return the in-memory KVS, since
// 	 * the stock faasd code already expects this return value */
// 	return DeployedFunctions, nil
// }

// // GetFunction returns a function that matches name
// func GetFunction(client *containerd.Client, name string, namespace string) (Function, error) {
// 	/* Quick and dirty code for maintaining the expected return type. This allows
// 	 * us to avoid having to modify other functions that call this one.
// 	 * We create an empty Function struct and populate it with values
// 	 * from DeployedFunctions[name]
// 	 * We also stub out unused values such as 'IP' and 'replicas' */
// 	/* TODO: Look into making this more elegant and updating functions
// 	 * that call this one */
// 	fn := Function{}
// 	if _, ok := DeployedFunctions[name]; ok {
// 		fn.name = name
// 		fn.namespace = DeployedFunctions[name].namespace
// 		fn.image = DeployedFunctions[name].image
// 		fn.pid = 0
// 		fn.replicas = len(DeployedFunctions[name].idleReplicas)
// 		fn.IP = "127.0.0.0"
// 		fn.labels = DeployedFunctions[name].labels
// 		fn.annotations = DeployedFunctions[name].annotations
// 		fn.secrets = DeployedFunctions[name].secrets
// 		fn.secretsPath = DeployedFunctions[name].secretsPath
// 		fn.envVars = DeployedFunctions[name].envVars
// 		fn.envProcess = DeployedFunctions[name].envProcess
// 		fn.memoryLimit = DeployedFunctions[name].memoryLimit
// 		fn.createdAt = DeployedFunctions[name].createdAt
// 		return fn, nil
// 	}

// 	return Function{}, fmt.Errorf("[GetFunction] Unable to find function '%s'", name)
// }

func readEnvFromProcessEnv(env []string) (map[string]string, string) {
	foundEnv := make(map[string]string)
	fprocess := ""
	for _, e := range env {
		kv := strings.Split(e, "=")
		if len(kv) == 1 {
			continue
		}

		if kv[0] == "PATH" {
			continue
		}

		if kv[0] == "fprocess" {
			fprocess = kv[1]
			continue
		}

		foundEnv[kv[0]] = kv[1]
	}

	return foundEnv, fprocess
}

func readSecretsFromMounts(mounts []specs.Mount) []string {
	secrets := []string{}
	for _, mnt := range mounts {
		x := strings.Split(mnt.Destination, "/var/openfaas/secrets/")
		if len(x) > 1 {
			secrets = append(secrets, x[1])
		}

	}
	return secrets
}

// buildLabelsAndAnnotations returns a separated list with labels first,
// followed by annotations by checking each key of ctrLabels for a prefix.
func buildLabelsAndAnnotations(ctrLabels map[string]string) (map[string]string, map[string]string) {
	labels := make(map[string]string)
	annotations := make(map[string]string)

	for k, v := range ctrLabels {
		if strings.HasPrefix(k, annotationLabelPrefix) {
			annotations[strings.TrimPrefix(k, annotationLabelPrefix)] = v
		} else {
			labels[k] = v
		}
	}

	return labels, annotations
}

func ListNamespaces(client *containerd.Client) []string {
	set := []string{}
	store := client.NamespaceService()
	namespaces, err := store.List(context.Background())
	if err != nil {
		log.Printf("Error listing namespaces: %s", err.Error())
		set = append(set, fecore.DefaultFunctionNamespace)
		return set
	}

	for _, namespace := range namespaces {
		labels, err := store.Labels(context.Background(), namespace)
		if err != nil {
			log.Printf("Error listing label for namespace %s: %s", namespace, err.Error())
			continue
		}

		if _, found := labels[pkg.NamespaceLabel]; found {
			set = append(set, namespace)
		}

		if !findNamespace(fecore.DefaultFunctionNamespace, set) {
			set = append(set, fecore.DefaultFunctionNamespace)
		}
	}

	return set
}

func findNamespace(target string, items []string) bool {
	for _, n := range items {
		if n == target {
			return true
		}
	}
	return false
}

func readMemoryLimitFromSpec(spec *specs.Spec) int64 {
	if spec.Linux == nil || spec.Linux.Resources == nil || spec.Linux.Resources.Memory == nil || spec.Linux.Resources.Memory.Limit == nil {
		return 0
	}
	return *spec.Linux.Resources.Memory.Limit
}
