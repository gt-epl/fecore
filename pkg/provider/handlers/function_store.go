package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	gocni "github.com/containerd/go-cni"
	fecore "github.gatech.edu/faasedge/fecore/pkg"
	cninetwork "github.gatech.edu/faasedge/fecore/pkg/cninetwork"
	"github.gatech.edu/faasedge/fecore/pkg/provider/config"
	"github.gatech.edu/faasedge/fecore/pkg/provider/storage"
	"github.gatech.edu/faasedge/fecore/pkg/service"
	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

/*
 * All read functions return copies and not references
 * All write functions should update the storage
 */

/* In-memory map for keeping track of functions that have been created */
type FunctionStore struct {
	deployedFunctions map[string]*Function
	functionStats     map[string]*FunctionStats
	storageManager    storage.StorageManager

	coldStartTimes      []int64
	invocationTimesCold []int64 // cold and warm distinguish how many retries
	invocationTimesWarm []int64

	cfg config.Config

	MAX_ADDL_CTRS      int
	MAX_KEEPALIVE_TIME int

	nextIP             net.IP
	containerCount     int64
	wasmContainerCount int64
	//ipconfigs map[string]string
	ipconfigs sync.Map

	/* Begin netns test */
	nsMu      sync.RWMutex
	nextNS    int64
	netnsList []wasmIPInfo
	nextNSIP  net.IP
	/* End netns test */

	statsChan chan FunctionStat

	/* Begin mutexes */
	mu        sync.RWMutex // Added a rw mutex and things like reading the whole map require a global map anyways, TODO check if there are other strategies
	metricMu  sync.RWMutex
	ipamMu    sync.RWMutex
	dfMu      sync.RWMutex
	ccMu      sync.RWMutex // containerCount mutex
	cleanupMu sync.RWMutex
	/* End mutexes */

	Client *containerd.Client
	CNI    *gocni.CNI
}

func InitFunctionStore(storageManager storage.StorageManager, fecoreConfig config.Config) (*FunctionStore, error) {
	fs := FunctionStore{
		/* Init mutexes */
		mu:        sync.RWMutex{},
		metricMu:  sync.RWMutex{},
		ipamMu:    sync.RWMutex{},
		nsMu:      sync.RWMutex{},
		dfMu:      sync.RWMutex{},
		ccMu:      sync.RWMutex{},
		cleanupMu: sync.RWMutex{},
		//ipconfigs:         make(map[string]string),
		deployedFunctions:  make(map[string]*Function),
		functionStats:      make(map[string]*FunctionStats),
		storageManager:     storageManager,
		ipconfigs:          sync.Map{},
		statsChan:          make(chan FunctionStat, 100),
		MAX_ADDL_CTRS:      5,
		MAX_KEEPALIVE_TIME: 60,
	}

	fs.cfg = fecoreConfig

	sFns, err := storageManager.GetAllFunctions()
	if err != nil {
		return nil, err
	}

	for _, sFn := range sFns {
		fn := getFunction(&sFn)
		cns, err := storageManager.GetContainersForFunction(fn.name)
		if err != nil {
			return nil, err
		}
		for _, c := range cns {
			replica := Replica{}
			replica.fname = fn.name
			replica.ctrType = "" // TODO: Populate this from DB
			replica.uuid = c.Name
			replica.PID = 1 // TODO: Populate this from DB
			replica.IP = c.Ip
			replica.lastAccess = time.Now()
			fs.AddIdleReplica(&replica)
		}
		fs.deployedFunctions[fn.name] = &fn
	}

	fs.nextIP = net.IPv4(10, 62, 0, 1)

	/* Begin netns test */
	fs.nextNS = 10
	fs.nextNSIP = net.IPv4(10, 63, 100, 10)
	/* End netns test */

	fs.containerCount = 0
	fs.wasmContainerCount = 0

	return &fs, nil
}

/**
 * Cleanup function replicas that are not being used
 */
func (fs *FunctionStore) CleanupDaemon(client *containerd.Client, cni gocni.CNI) {
	if !fs.cleanupMu.TryLock() {
		return
	}
	defer fs.cleanupMu.Unlock()

	currTs := time.Now()

	for name, fn := range fs.deployedFunctions {
		var cleanupCount int
		fn.idleReplicasLock.Lock()

		for cleanupCount = 0; fn.idleReplicas.LRU != nil; cleanupCount++ {
			replica := fn.idleReplicas.LRU
			if currTs.Sub(replica.lastAccess).Seconds() >= float64(fs.cfg.ContainerExpirationTime) {
				timec.LogEvent("function_store/CleanupDaemon", fmt.Sprintf("Removing expired replica '%s'", replica.uuid), 2)
				// Remove from idle replicas list
				fs.RemoveIdleReplica(fn.name, "Cleanup")
				fn.idleReplicasLock.Unlock()
				// Delete the actual replica container
				fs.DeleteReplica(replica)
				// Grace period to avoid bogging down the system with deletes
				time.Sleep(10 * time.Millisecond)
				fn.idleReplicasLock.Lock()
			} else {
				break
			}
		}
		// See if MRU expired
		if fn.idleReplicas.MRU != nil {
			replica := fn.idleReplicas.MRU
			if currTs.Sub(replica.lastAccess).Seconds() >= float64(fs.cfg.ContainerExpirationTime) {
				timec.LogEvent("function_store/CleanupDaemon", fmt.Sprintf("Removing expired MRU replica '%s'", replica.uuid), 2)
				fn.idleReplicas.MRU = nil
				fs.DeleteReplica(replica)
			}
		}

		fn.idleReplicasLock.Unlock()
		if cleanupCount > 0 {
			timec.LogEvent("function_store/CleanupDaemon", fmt.Sprintf("Cleaned up %d expired replicas for Function '%s'", cleanupCount, name), 2)
		}
	}
}

/* Gets a function's metadata from DB, creates/populates Function struct
 * and returns pointer to that struct.
 */
func (fs *FunctionStore) GetDeployedFunctions() ([]*Function, error) {
	fs.dfMu.RLock()
	defer fs.dfMu.RUnlock()
	fns := make([]*Function, 0)
	for _, value := range fs.deployedFunctions {
		fns = append(fns, value)
	}
	return fns, nil
}

/* Populates a caller-provided Function struct with a copy of metadata for a
 * deployed Function */
func (fs *FunctionStore) GetDeployedFunction(name string, tmp *Function, requestID string) (err error) {
	defer timec.RecordDuration("(function_store.go) GetDeployedFunction() <requestID="+requestID+">", time.Now())
	fs.dfMu.RLock()
	defer fs.dfMu.RUnlock()
	if fn, ok := fs.deployedFunctions[name]; ok {
		tmp.name = fn.name
		tmp.namespace = fn.namespace
		tmp.image = fn.image
		tmp.imageFiles = fn.imageFiles
		tmp.pid = fn.pid
		tmp.replicas = fn.replicas
		tmp.labels = fn.labels
		tmp.sandboxes = fn.sandboxes
		tmp.annotations = fn.annotations
		tmp.secrets = fn.secrets
		tmp.secretsPath = fn.secretsPath
		tmp.envVars = fn.envVars
		tmp.envProcess = fn.envProcess
		tmp.memoryLimit = fn.memoryLimit
		tmp.createdAt = fn.createdAt
		return nil
	}
	return fmt.Errorf("[GetDeployedFunction] Unable to get function '%s' from FunctionStore", name)
}

/* Add deployed function to the store */
func (fs *FunctionStore) AddDeployedFunction(fn *Function) error {
	err := fs.storageManager.InsertFunction(getStorageFunction(fn))
	if err != nil {
		return err
	}
	for _, c := range getContainers(fn) {
		err = fs.storageManager.InsertContainer(c)
		if err != nil {
			return err
		}
	}
	fs.dfMu.Lock()
	fs.deployedFunctions[fn.name] = fn
	fs.dfMu.Unlock()

	/* Begin init stats for this fn */
	fnStats := FunctionStats{}
	fnStats.entryPos = 0
	fnStats.coldPos = 0
	fnStats.warmPos = 0
	fnStats.invokeNext = "native"
	fnStats.avgStartupTime = 0
	fnStats.avgExecTime = 0
	fnStats.avgSvcTime = 0
	fnStats.totalStartupTime = 0
	fnStats.totalExecTime = 0
	fnStats.totalSvcTime = 0
	fnStats.totalInvocations = 0
	fnStats.currInvocations = 0
	fnStats.warmStarts = 0
	fnStats.coldStarts = 0
	fnStats.activeCount = 0
	fnStats.idleCount = 0
	fnStats.statMu = sync.RWMutex{}
	fs.functionStats[fn.name] = &fnStats
	/* End init stats for this fn */

	timec.LogEvent("function_store/AddDeployedFunction", fmt.Sprintf("Added deployed Function '%s' in namespace '%s'", fn.name, fn.namespace), 2)
	return nil
}

/* Updates a function's metadata in DB */
func (fs *FunctionStore) UpdateDeployedFunction(fn *Function) error {
	timec.LogEvent("function_store/UpdateDeployedFunction", fmt.Sprintf("Updated metadata for deployed Function '%s'", fn.name), 2)
	return nil
}

/* Remove function from store and db */
func (fs *FunctionStore) RemoveDeployedFunction(name string) error {
	fs.dfMu.RLock()
	fn := fs.deployedFunctions[name]
	fs.dfMu.RUnlock()

	err := fs.storageManager.DeleteFunction(name)
	if err != nil {
		return err
	}
	for _, c := range getContainers(fn) {
		fs.storageManager.DeleteContainer(c.Name)
	}

	fs.dfMu.Lock()
	defer fs.dfMu.Unlock()
	if _, ok := fs.deployedFunctions[name]; ok {
		delete(fs.deployedFunctions, name)
		timec.LogEvent("function_store/RemoveDeployedFunction", fmt.Sprintf("Removed deployed Function '%s'", name), 2)
		return nil
	}
	return fmt.Errorf("[function_store/RemoveDeployedFunction] Unable to locate function %s", name)
}

func (fs *FunctionStore) GetFunctionLabels(name string) (labels map[string]string, err error) {
	fs.dfMu.RLock()
	defer fs.dfMu.RUnlock()
	if fn, ok := fs.deployedFunctions[name]; ok {
		return fn.labels, nil
	}
	return nil, fmt.Errorf("[GetFunctionLabels] Unable to get labels for Function '%s'", name)
}

func (fs *FunctionStore) GetFunctionImage(name string) (image string, imageFiles []string, err error) {
	fs.dfMu.RLock()
	defer fs.dfMu.RUnlock()
	if fn, ok := fs.deployedFunctions[name]; ok {
		return fn.image, fn.imageFiles, nil
	}
	return "", nil, fmt.Errorf("[GetFunctionImage] Unable to get image name for Function '%s'", name)
}

/* Add a Replica to the collection of IdleReplicas for a Function */
func (fs *FunctionStore) AddIdleReplica(replica *Replica) error {
	replica.lastAccess = time.Now()
	fs.deployedFunctions[replica.fname].idleReplicasLock.Lock()
	defer fs.deployedFunctions[replica.fname].idleReplicasLock.Unlock()

	if fs.deployedFunctions[replica.fname].idleReplicas.MRU == nil {
		fs.deployedFunctions[replica.fname].idleReplicas.MRU = replica
		fs.deployedFunctions[replica.fname].idleReplicas.count += 1
		return nil
	}

	if fs.deployedFunctions[replica.fname].idleReplicas.LRU == nil &&
		len(fs.deployedFunctions[replica.fname].idleReplicas.containers) == 0 {
		fs.deployedFunctions[replica.fname].idleReplicas.LRU = fs.deployedFunctions[replica.fname].idleReplicas.MRU
		fs.deployedFunctions[replica.fname].idleReplicas.MRU = replica
		fs.deployedFunctions[replica.fname].idleReplicas.count += 1
		return nil
	}

	// Push current MRU to containers[] slice
	fs.deployedFunctions[replica.fname].idleReplicas.containers = append(fs.deployedFunctions[replica.fname].idleReplicas.containers, fs.deployedFunctions[replica.fname].idleReplicas.MRU)
	// Replace current MRU with replica
	fs.deployedFunctions[replica.fname].idleReplicas.MRU = replica
	fs.deployedFunctions[replica.fname].idleReplicas.count += 1
	return nil
}

/* Gets the container name of an idle (warm) Function replica and sets the
 * container replica to Active */
func (fs *FunctionStore) GetIdleReplica(fn string, requestID string) (string, string, error) {
	defer timec.RecordDuration("(function_store.go) GetIdleReplica() <requestID="+requestID+">", time.Now())
	timec.LogEvent("function_store/GetIdleReplica", fmt.Sprintf("Looking for idle replica for '%s' <requestID=%s>", fn, requestID), 2)
	fs.deployedFunctions[fn].idleReplicasLock.Lock()
	defer fs.deployedFunctions[fn].idleReplicasLock.Unlock()

	if fs.deployedFunctions[fn].idleReplicas.MRU == nil {
		timec.LogEvent("function_store/GetIdleReplica", fmt.Sprintf("No idle replicas available for '%s' <requestID=%s>", fn, requestID), 2)
		return "", "", fmt.Errorf("[function_store/GetIdleReplica] No idle replicas available for '%s' <requestID=%s>", fn, requestID)
	}

	var name string
	var ip string
	recycledContainer := fs.deployedFunctions[fn].idleReplicas.MRU
	name = recycledContainer.uuid
	ip = recycledContainer.IP

	fs.deployedFunctions[fn].activeReplicasLock.Lock()
	fs.deployedFunctions[fn].activeReplicas[name] = recycledContainer
	fs.deployedFunctions[fn].activeReplicasLock.Unlock()

	idleReplicasLen := len(fs.deployedFunctions[fn].idleReplicas.containers)
	if idleReplicasLen > 0 {
		// If there's still a container in the slice, assign it to MRU
		fs.deployedFunctions[fn].idleReplicas.MRU = fs.deployedFunctions[fn].idleReplicas.containers[idleReplicasLen-1]
		fs.deployedFunctions[fn].idleReplicas.count -= 1
		fs.deployedFunctions[fn].idleReplicas.containers = fs.deployedFunctions[fn].idleReplicas.containers[:idleReplicasLen-1]
	} else {
		// Otherwise, assign MRU to nil since there are no more idle containers
		fs.deployedFunctions[fn].idleReplicas.MRU = nil
		fs.deployedFunctions[fn].idleReplicas.count = 0
	}

	return name, ip, nil
}

/* Removes idle replica from idleReplicas.containers[]
 * 	- Caller must hold idleReplicasLock when calling this function
 * 	- Caller must call DeleteReplica() to fully remove the replica */
func (fs *FunctionStore) RemoveIdleReplica(fn string, requestID string) {

	if len(fs.deployedFunctions[fn].idleReplicas.containers) > 0 {
		fs.deployedFunctions[fn].idleReplicas.LRU = fs.deployedFunctions[fn].idleReplicas.containers[0]
		fs.deployedFunctions[fn].idleReplicas.containers = fs.deployedFunctions[fn].idleReplicas.containers[1:]
		fs.deployedFunctions[fn].idleReplicas.count -= 1
	} else {
		fs.deployedFunctions[fn].idleReplicas.LRU = nil
		fs.deployedFunctions[fn].idleReplicas.count = 0
	}
}

/* Add a Replica to the collection of ActiveReplicas for a Function */
func (fs *FunctionStore) AddActiveReplica(replica *Replica) error {
	fs.deployedFunctions[replica.fname].activeReplicasLock.Lock()
	defer fs.deployedFunctions[replica.fname].activeReplicasLock.Unlock()
	fs.deployedFunctions[replica.fname].activeReplicas[replica.uuid] = replica

	fs.functionStats[replica.fname].statMu.Lock()
	fs.functionStats[replica.fname].activeCount += 1
	fs.functionStats[replica.fname].statMu.Unlock()

	return nil
}

/* Deletes all Replicas for a Function */
func (fs *FunctionStore) DeleteAllReplicas(ctx context.Context, client *containerd.Client, cni gocni.CNI, fn string) error {
	/* Delete Idle Replicas */
	fs.deployedFunctions[fn].idleReplicasLock.Lock()
	defer fs.deployedFunctions[fn].idleReplicasLock.Unlock()
	// TODO: Update this so we handle empty idleReplicas more gracefully
	if fs.deployedFunctions[fn].idleReplicas.count <= 0 {
		return nil
	}
	// Delete all Replicas stored in containers slice
	for _, replica := range fs.deployedFunctions[fn].idleReplicas.containers {
		switch replica.ctrType {
		case "native":
			fs.DeleteNativeReplica(ctx, client, cni, replica)
		case "wasm":
			fs.DeleteWasmReplica(ctx, client, cni, replica)
		default:
			timec.LogEvent("function_store/DeleteAllReplicas", fmt.Sprintf("Could not find matching delete operation for Replica '%s' with ctrType '%s'\n", replica.uuid, replica.ctrType), 1)
		}
	}
	/* TODO: Code for Delete Active Replicas */
	/* One consideration: How do we handle response to clients if we kill an active replica before it finishes execution? */
	return nil
}

/* Updates active/idle replica counts for Hybrid deployments */
func (fs *FunctionStore) UpdateReplicaCount(fn string, status string, count int) {
	var currActive int
	var currIdle int
	switch status {
	case "idle":
		fs.functionStats[fn].statMu.Lock()
		fs.functionStats[fn].idleCount += count
		currIdle = fs.functionStats[fn].idleCount
		currActive = fs.functionStats[fn].activeCount
		fs.functionStats[fn].statMu.Unlock()
	case "active":
		fs.functionStats[fn].statMu.Lock()
		fs.functionStats[fn].activeCount += count
		currIdle = fs.functionStats[fn].idleCount
		currActive = fs.functionStats[fn].activeCount
		fs.functionStats[fn].statMu.Unlock()
	}
	timec.LogEvent("function_store/UpdateReplicaCount", fmt.Sprintf("<%s> active=%d, idle=%d", fn, currActive, currIdle), 3)
}

func (fs *FunctionStore) AddWasmContainerCount() bool {
	fs.ccMu.Lock()
	defer fs.ccMu.Unlock()
	if fs.wasmContainerCount < int64(fs.cfg.MaxWasmContainers) {
		fs.wasmContainerCount += 1
		timec.LogEvent("function_store/AddWasmContainerCount", fmt.Sprintf("WASM container count: %d", fs.wasmContainerCount), 3)
		return true
	}
	timec.LogEvent("function_store/AddWasmContainerCount", fmt.Sprintf("WASM container count: %d", fs.wasmContainerCount), 3)
	return false
}

func (fs *FunctionStore) AddContainerCount() bool {
	fs.ccMu.Lock()
	defer fs.ccMu.Unlock()
	if fs.containerCount < 1000 {
		fs.containerCount += 1
		timec.LogEvent("function_store/AddContainerCount", fmt.Sprintf("Native container count: %d", fs.containerCount), 3)
		return true
	}
	timec.LogEvent("function_store/AddContainerCount", fmt.Sprintf("Native container count: %d", fs.containerCount), 3)
	return false
}

func (fs *FunctionStore) DelContainerCount() bool {
	fs.ccMu.Lock()
	defer fs.ccMu.Unlock()
	if fs.containerCount > 0 {
		fs.containerCount -= 1
		timec.LogEvent("function_store/DelContainerCount", fmt.Sprintf("Native container count: %d", fs.containerCount), 3)
		return true
	}
	return false
}

func (fs *FunctionStore) DelWasmContainerCount() bool {
	fs.ccMu.Lock()
	defer fs.ccMu.Unlock()
	if fs.wasmContainerCount > 0 {
		fs.wasmContainerCount -= 1
		timec.LogEvent("function_store/DelWasmContainerCount", fmt.Sprintf("WASM container count: %d", fs.wasmContainerCount), 3)
		return true
	}
	return false
}

func (fs *FunctionStore) DeleteReplica(replica *Replica) error {
	ctx := namespaces.WithNamespace(context.Background(), fecore.DefaultFunctionNamespace)
	cni := *fs.CNI
	client := fs.Client
	switch replica.ctrType {
	case "native":
		fs.DeleteNativeReplica(ctx, client, cni, replica)
	case "wasm":
		fs.DeleteWasmReplica(ctx, client, cni, replica)
	default:
		timec.LogEvent("function_store/DeleteReplica", fmt.Sprintf("Could not find matching delete operation for Replica '%s' with ctrType '%s'\n", replica.uuid, replica.ctrType), 1)
	}
	return nil
}

func (fs *FunctionStore) DeleteNativeReplica(ctx context.Context, client *containerd.Client, cni gocni.CNI, replica *Replica) error {
	name := replica.uuid
	networkErr := cninetwork.DeleteCNINetwork(ctx, cni, client, name)
	if networkErr != nil {
		timec.LogEvent("function_store/DeleteNativeReplica", fmt.Sprintf("Error removing network for Function '%s': %s", name, networkErr), 1)
	}
	containerErr := service.Remove(ctx, client, name)
	if containerErr != nil {
		timec.LogEvent("function_store/DeleteNativeReplica", fmt.Sprintf("Error removing replica container '%s': %s", name, containerErr), 1)
		return containerErr
	}
	fs.DelContainerCount()
	return nil
}

func (fs *FunctionStore) DeleteWasmReplica(ctx context.Context, client *containerd.Client, cni gocni.CNI, replica *Replica) error {
	fn := replica.fname
	image := fs.deployedFunctions[fn].image
	name := replica.uuid
	pid := int(replica.PID)
	networkErr := cninetwork.DeleteWasmCNINetwork(ctx, cni, client, replica.uuid, int(pid))
	if networkErr != nil {
		timec.LogEvent("function_store/DeleteWasmReplica", fmt.Sprintf("Error removing network for Function '%s': %s", name, networkErr), 1)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		timec.LogEvent("function_store/DeleteWasmReplica", fmt.Sprintf("Could not find WASM container process (PID=%d)", pid), 1)
		return err
	} else {
		proc.Kill()
		timec.LogEvent("function_store/DeleteWasmReplica", fmt.Sprintf("Killed WASM container process (PID=%d)", pid), 2)
	}
	err = os.RemoveAll(image + "/replicas/" + name)
	if err != nil {
		timec.LogEvent("function_store/DeleteWasmReplica", fmt.Sprintf("Could not delete rootfs for WASM replica '%s'", name), 1)
		return err
	} else {
		timec.LogEvent("function_store/DeleteWasmReplica", fmt.Sprintf("Deleted rootfs for WASM replica '%s'", name), 2)
	}

	/* Remove wasm replica's cgroups */
	wasm_cg_cpu_path := "/sys/fs/cgroup/cpu/fewasm"
	wasm_cg_mem_path := "/sys/fs/cgroup/memory/fewasm"

	/* Add wasmPid to its own cpu cgroup */
	cg_cpu_err := os.RemoveAll(wasm_cg_cpu_path + "/" + name)
	if cg_cpu_err != nil {
		timec.LogEvent("function_store/DeleteWasmReplica", fmt.Sprintf("Unable to remove CPU cgroup for %s", name), 1)
	} else {
		timec.LogEvent("function_store/DeleteWasmReplica", fmt.Sprintf("Deleted CPU cgroup for %s", name), 2)
	}
	cg_mem_err := os.RemoveAll(wasm_cg_mem_path + "/" + name)
	if cg_mem_err != nil {
		timec.LogEvent("function_store/DeleteWasmReplica", fmt.Sprintf("Unable to remove MEMORY cgroup for %s", name), 1)
	} else {
		timec.LogEvent("function_store/DeleteWasmReplica", fmt.Sprintf("Deleted MEMORY cgroup for %s", name), 2)
	}

	fs.DelWasmContainerCount()
	return nil
}

/* Change container from active to inactive
 * Note: lookup by ip and container name is not avaiable in resolver
 */
func (fs *FunctionStore) UpdateReplicaStatusInactive(fn string, replicaName string, requestID string) error {
	defer timec.RecordDuration("(function_store.go) UpdateReplicaStatusInactive() Returning replica "+replicaName+" to inactive pool <requestID="+requestID+">", time.Now())

	var replicaType string
	var effectiveFname string
	effectiveFname = fn
	if strings.Contains(replicaName, "_n") {
		replicaType = "native"
	}
	if strings.Contains(replicaName, "_w") {
		replicaType = "wasm"
	}
	fs.dfMu.RLock()
	ftype := fs.deployedFunctions[fn].labels["ctrType"]
	if ftype == "hybrid" {
		effectiveFname = fs.deployedFunctions[fn].sandboxes[replicaType]
		timec.LogEvent("function_store/UpdateReplicaStatusInactive", fmt.Sprintf("Function type is Hybrid; replicaType is %s; effectiveFname is %s; replicaName is %s <requestID=%s>", replicaType, effectiveFname, replicaName, requestID), 3)
	}
	fs.dfMu.RUnlock()

	fs.deployedFunctions[effectiveFname].activeReplicasLock.Lock()
	replica := fs.deployedFunctions[effectiveFname].activeReplicas[replicaName]
	delete(fs.deployedFunctions[effectiveFname].activeReplicas, replicaName)
	fs.deployedFunctions[effectiveFname].activeReplicasLock.Unlock()

	policy := fs.GetInvocationPolicy(fn)
	if policy.keepaliveColdStartCtr != 0 && replica.ctrType == policy.coldStartCtrType {
		timec.LogEvent("function_store/UpdateReplicaStatusInactive", fmt.Sprintf("replica.ctrType = %s (coldStartCtrType=%s; warmStartCtrType=%s) - killing <requestID=%s>", replica.ctrType, policy.coldStartCtrType, policy.warmStartCtrType, requestID), 4)
		fs.DeleteReplica(replica)
		return nil
	} else {
		keepaliveStatus := "false"
		if policy.keepaliveColdStartCtr != 0 {
			keepaliveStatus = "true"
		}
		timec.LogEvent("function/UpdateReplicaStatusInactive", fmt.Sprintf("Did not remove container for <requestID=%s>: policy.keepaliveColdStartCtr=%s, policy.coldStartCtrType=%s, policy.warmStartCtrType=%s", requestID, keepaliveStatus, policy.coldStartCtrType, policy.warmStartCtrType), 4)
	}
	fs.AddIdleReplica(replica)
	return nil
}

func (fs *FunctionStore) GetNetNS(requestID string) (int, string) {
	fs.nsMu.Lock()
	defer fs.nsMu.Unlock()
	defer timec.RecordDuration("(function_store.go).GetNetNS <requestID="+requestID+">", time.Now())
	if len(fs.netnsList) == 0 {
		return -1, ""
	}
	ipInfo := fs.netnsList[0]
	thisNS := ipInfo.netnsNum
	thisIP := ipInfo.IP
	fs.netnsList = fs.netnsList[1:]
	timec.LogEvent("function_store/GetNetNS", fmt.Sprintf("Retrieving WASM ns=%d, IP=%s from pool <requestID=%s>", thisNS, thisIP, requestID), 2)
	return thisNS, thisIP
}

func (fs *FunctionStore) ReturnNetNS(nsNum int, IP string) {
	fs.nsMu.Lock()
	defer fs.nsMu.Unlock()
	ipInfo := wasmIPInfo{}
	ipInfo.netnsNum = nsNum
	ipInfo.IP = IP
	fs.netnsList = append(fs.netnsList, ipInfo)
	timec.LogEvent("function_store/ReturnNetNS", fmt.Sprintf("Returning WASM ns=%d, IP=%s to pool", nsNum, IP), 2)
}

/* Utility functions */
func getFunction(f *storage.Function) Function {
	labels := map[string]string{}
	json.Unmarshal([]byte(f.Labels), &labels)

	annotations := map[string]string{}
	json.Unmarshal([]byte(f.Annotations), &annotations)

	envVars := map[string]string{}
	json.Unmarshal([]byte(f.EnvVars), &annotations)

	secrets := []string{}
	json.Unmarshal([]byte(f.Secrets), &annotations)

	return Function{
		name:           f.Name,
		namespace:      f.Namespace,
		image:          f.Image,
		labels:         labels,
		idleReplicas:   IdleReplicas{},
		idleReplicasTs: make(map[string]time.Time),
		activeReplicas: make(map[string]*Replica),
		annotations:    annotations,
		secrets:        secrets,
		secretsPath:    f.SecretsPath,
		envVars:        envVars,
		envProcess:     f.EnvProcess,
		memoryLimit:    f.MemoryLimit,
	}
}

func getStorageFunction(f *Function) storage.Function {
	lables, _ := json.Marshal(f.labels)
	annotations, _ := json.Marshal(f.annotations)
	envVars, _ := json.Marshal(f.envVars)
	secrets, _ := json.Marshal(f.secrets)

	return storage.Function{
		Name:        f.name,
		Namespace:   f.namespace,
		Image:       f.image,
		Labels:      string(lables),
		Annotations: string(annotations),
		Secrets:     string(secrets),
		SecretsPath: f.secretsPath,
		EnvVars:     string(envVars),
		EnvProcess:  f.envProcess,
		MemoryLimit: f.memoryLimit,
	}
}

func getContainers(f *Function) []storage.Container {
	var cns []storage.Container
	for cname, replica := range f.activeReplicas {
		cns = append(cns, storage.Container{Name: cname, ParentFunction: f.name, Ip: replica.IP})
	}
	for _, replica := range f.idleReplicas.containers {
		cns = append(cns, storage.Container{Name: replica.uuid, ParentFunction: f.name, Ip: replica.IP})
	}
	return cns
}
