package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/KarpelesLab/reflink"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	gocni "github.com/containerd/go-cni"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/openfaas/faas-provider/types"
	"github.gatech.edu/faasedge/fecore/pkg/service"

	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

func MakeReplicaReaderHandler(client *containerd.Client, fs *FunctionStore) func(w http.ResponseWriter, r *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		functionName := vars["name"]
		lookupNamespace := getRequestNamespace(readNamespaceFromQuery(r))

		// Check if namespace exists, and it has the proper label
		valid, err := validNamespace(client.NamespaceService(), lookupNamespace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if !valid {
			http.Error(w, "namespace not valid", http.StatusBadRequest)
			return
		}

		/* TODO: Look into how 'found' JSON is used by gateway to call Functions
		 * (esp. wrt the 'AvailableReplicas' field).
		 * For now, hardcoding AvailableReplicas to a high number tricks the
		 * Gateway's scaling logic into sending over the request w/o delay.
		 * We want to implement explicit scaling requests in the future, but
		 * for now we'll handle scaling interally via invoke_resolver.go */
		f := Function{}
		if err := fs.GetDeployedFunction(functionName, &f, "ReplicaHandler"); err == nil {
			found := types.FunctionStatus{
				Name:              functionName,
				Image:             f.image,
				AvailableReplicas: 99,
				Replicas:          uint64(f.replicas + 99),
				Namespace:         f.namespace,
				Labels:            &f.labels,
				Annotations:       &f.annotations,
				Secrets:           f.secrets,
				EnvVars:           f.envVars,
				EnvProcess:        f.envProcess,
				CreatedAt:         f.createdAt,
			}

			functionBytes, _ := json.Marshal(found)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(functionBytes)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func createReplica(fs *FunctionStore, client *containerd.Client, cni gocni.CNI, fname string, ctrType string, setActive bool, requestID string) (replicaName string, replicaIP string, err error) {
	sleepTime := 0
	proceed := false
	for sleepTime < 60000 { // retry for 60 sec
		if ctrType == "wasm" {
			proceed = fs.AddWasmContainerCount()
		} else {
			proceed = fs.AddContainerCount()
		}
		if proceed {
			break
		} else {
			time.Sleep(time.Duration(100) * time.Millisecond)
			sleepTime += 100
		}
	}
	if !proceed {
		return "", "", fmt.Errorf("container limit reached")
	}

	if ctrType == "native" {
		replicaName, replicaIP, err = createNativeReplica(client, cni, fs, fname, requestID, setActive)
	} else if ctrType == "wasm" {
		replicaName, replicaIP, err = createWasmReplica(fname, fs, requestID, setActive)
	}

	if err != nil {
		return "", "", err
	}
	return replicaName, replicaIP, nil
}

func createNativeReplica(client *containerd.Client, cni gocni.CNI, fs *FunctionStore, fname string, requestID string, setActive bool) (replicaName string, replicaIP string, err error) {
	defer timec.RecordDuration("(replicas.go).createReplica <requestID="+requestID+">", time.Now())

	fn := Function{}
	err = fs.GetDeployedFunction(fname, &fn, requestID)

	if err != nil {
		return "", "", err
	}
	ctx := namespaces.WithNamespace(context.Background(), fn.namespace)

	// snapshotter := "overlay"
	snapshotter := ""
	if val, ok := os.LookupEnv("snapshotter"); ok {
		snapshotter = val
	}

	image, err := service.PrepareImage(ctx, client, fn.image, requestID, snapshotter, false)
	if err != nil {
		return "", "", fmt.Errorf("[createReplica] Unable to pull image %s, %w", fn.image, err)
	}

	envs := prepareEnv(fn.envProcess, fn.envVars)

	mounts := getOSMounts()

	for _, secret := range fn.secrets {
		mounts = append(mounts, specs.Mount{
			Destination: path.Join("/var/openfaas/secrets", secret),
			Type:        "bind",
			Source:      path.Join(fn.secretsPath, secret),
			Options:     []string{"rbind", "ro"},
		})
	}
	uuid := uuid.New().String()
	name := fn.name + "_" + uuid + "_n"

	labels := fn.labels

	//var memory *specs.LinuxMemory
	var memory = &specs.LinuxMemory{}
	memory.Limit = &fn.memoryLimit

	container, err := client.NewContainer(
		ctx,
		name,
		// requestID,
		containerd.WithImage(image),
		containerd.WithSnapshotter(snapshotter),
		containerd.WithNewSnapshot(name+"-snapshot", image),
		containerd.WithNewSpec(oci.WithImageConfig(image),
			oci.WithHostname(name),
			oci.WithCapabilities([]string{"CAP_NET_RAW"}),
			oci.WithMounts(mounts),
			oci.WithAnnotations(labels),
			oci.WithEnv(envs),
			oci.WithCPUShares(1024),
			oci.WithCPUs("5-15"),
			withMemory(memory)),
		containerd.WithContainerLabels(labels),
	)

	if err != nil {
		return "", "", fmt.Errorf("[createReplica] Unable to create container '%s': %w", name, err)
	}

	ip, createTaskStatus := createTask(ctx, container, requestID, cni)
	if createTaskStatus != nil {
		return "", "", fmt.Errorf("[createReplica] Unable to create task for container '%s': %w", name, createTaskStatus)
	}
	task, err := container.Task(ctx, nil)
	if err == nil {
		// Task for container exists
		_, err := task.Status(ctx)
		if err != nil {
			return "", "", fmt.Errorf("[createReplica] Unable to get task status for container '%s': %w", name, err)
		}
		/* Create a Replica for this Function instance */
		replica := Replica{}
		replica.fname = fn.name
		replica.ctrType = "native"
		replica.uuid = name
		replica.PID = task.Pid()
		replica.IP = ip
		replica.lastAccess = time.Now()

		if setActive {
			fs.AddActiveReplica(&replica)
		} else {
			fs.AddIdleReplica(&replica)
		}
		timec.LogEvent("replicas/createNativeReplica", fmt.Sprintf("Created native container for Function '%s' <requestID=%s>", name, requestID), 2)
		return replica.uuid, replica.IP, nil
	}
	return "", "", err
}

func setupWasmStorage(fs *FunctionStore, fname string, replicaName string, requestID string) (image string, err error) {
	defer timec.RecordDuration("(replicas.go).setupWasmStorage <requestID="+requestID+">", time.Now())
	image_path, imageFiles, err := fs.GetFunctionImage(fname)
	if err != nil {
		return "", err
	}

	rootfs_path := image_path + "/replicas/" + replicaName
	/* Check if unique dir for replica exists; if so, remove */
	if _, err := os.Stat(rootfs_path); !os.IsNotExist(err) {
		timec.LogEvent("[replicas/setupWasmStorage]", fmt.Sprintf("WASM replica path already exists at '%s'. Removing", rootfs_path), 1)
		os.RemoveAll(rootfs_path)
	}
	/* Create rootfs dir for Function instance */
	err = os.Mkdir(rootfs_path, 0744)
	if err != nil {
		timec.LogEvent("[replicas/setupWasmStorage]", fmt.Sprintf("Unable to create rootfs for WASM container '%s'", replicaName), 1)
		return "", err
	}

	/* Reflink all files in image rootfs */
	for _, file := range imageFiles {
		err := reflink.Auto(image_path+"/rootfs/"+file, rootfs_path+"/"+file)
		if err != nil {
			timec.LogEvent("[replicas/setupWasmStorage]", fmt.Sprintf("Error creating reflink to file '%s' for container '%s'", file, replicaName), 1)
			return "", err
		}
	}
	timec.LogEvent("[replicas/setupWasmStorage]", fmt.Sprintf("Setup WASM replica storage at '%s'", rootfs_path), 2)
	return image_path, nil
}

func createWasmReplica(fname string, fs *FunctionStore, requestID string, setActive bool) (replicaName string, replicaIP string, err error) {
	defer timec.RecordDuration("(replicas.go).createWasmReplica <requestID="+requestID+">", time.Now())

	labels, err := fs.GetFunctionLabels(fname)
	if err != nil {
		return "", "", err
	}
	/* Generate UUID */
	replicaName = fname + "_" + uuid.New().String() + "_w"
	var image string
	if val, ok := labels["ctrType"]; ok && (val == "hybrid") {
		image, err = setupWasmStorage(fs, fname+".wasm", replicaName, requestID)
	} else {
		/* Create unique dir for replica and setup reflinks */
		image, err = setupWasmStorage(fs, fname, replicaName, requestID)
	}
	if err != nil {
		return "", "", err
	}
	/* Get the next available network namespace */
	netnsNum, IP := fs.GetNetNS(requestID)
	if netnsNum == -1 {
		return "", "", fmt.Errorf("[replicas/createWasmReplica] No WASM network namespaces/IPs available")
	}
	/* Exec wasmedge to create container process, retrieve PID */
	runw_path := image + "/" + "runw"
	container_wasm_file := image + "/" + "function.wasm"
	container_dir_arg := ".:" + image + "/replicas/" + replicaName
	timec.LogEvent("replicas/createWasmReplica", fmt.Sprintf("Creating replica with command: /mnt/faasedge/runw %s %s %d <requestID=%s>", container_wasm_file, container_dir_arg, netnsNum, requestID), 2)
	startTime := time.Now()
	cmd := exec.Command(runw_path, container_wasm_file, container_dir_arg, strconv.Itoa(netnsNum))
	endTime := time.Since(startTime)
	timec.LogEvent("replicas/createWasmReplica/exec.Command", fmt.Sprintf("exec.Command() to start runw took %d ms <requestID=%s>", endTime.Milliseconds(), requestID), 2)

	startTime = time.Now()
	err = cmd.Start()
	endTime = time.Since(startTime)
	timec.LogEvent("replicas/createWasmReplica/cmd.Start", fmt.Sprintf("cmd.Start() to start runw took %d ms <requestID=%s>", endTime.Milliseconds(), requestID), 2)

	timec.RecordDuration("(replicas.go).exec.Command <requestID="+requestID+">", startTime)
	if err != nil {
		timec.LogEvent("replicas/createWasmReplica", fmt.Sprintf("Failed to start WASM container '%s' (%s)", replicaName, IP), 1)
		return "", "", err
	}

	wasmPid := cmd.Process.Pid

	/* Add wasmPid to cpuset cgroups */
	wasm_cg_cpuset_path := "/sys/fs/cgroup/cpuset/fewasm"
	wasm_cg_cpu_path := "/sys/fs/cgroup/cpu/fewasm"
	wasm_cg_mem_path := "/sys/fs/cgroup/memory/fewasm"
	wferr := os.WriteFile(filepath.Join(wasm_cg_cpuset_path, "tasks"), []byte(strconv.Itoa(wasmPid)), 0644)
	if wferr != nil {
		timec.LogEvent("replicas/CreateWasmReplica", fmt.Sprintf("Failed to write to CPUSET cgroup: %s <requestID=%s>", wferr, requestID), 1)
	}
	/* Add wasmPid to its own cpu cgroup */
	/* By default, each task gets 1 CPU in periods of contention, so we don't need to
	 * set the number of shares */
	os.Mkdir(filepath.Join(wasm_cg_cpu_path, replicaName), 0644)
	wferr = os.WriteFile(filepath.Join(wasm_cg_cpu_path, replicaName, "tasks"), []byte(strconv.Itoa(wasmPid)), 0644)
	/* Add quota to throttle function to 1 CPU even if no contention exists */
	// wferr = os.WriteFile(filepath.Join(wasm_cg_cpu_path, replicaName, "cpu.cfs_quota_us"), []byte(strconv.Itoa(100000)), 0644)
	if wferr != nil {
		timec.LogEvent("replicas/CreateWasmReplica", fmt.Sprintf("Failed to write to CPU cgroup: %s <requestID=%s>", wferr, requestID), 1)
	}
	/* Add wasmPid to its own memory cgroup */
	os.Mkdir(filepath.Join(wasm_cg_mem_path, replicaName), 0644)
	wferr = os.WriteFile(filepath.Join(wasm_cg_mem_path, replicaName, "tasks"), []byte(strconv.Itoa(wasmPid)), 0644)
	/* Limit each replica to 1 GB memory */
	// wferr = os.WriteFile(filepath.Join(wasm_cg_mem_path, replicaName, "memory.limit_in_bytes"), []byte(strconv.Itoa(1024*1024*1024)), 0644)
	if wferr != nil {
		timec.LogEvent("replicas/CreateWasmReplica", fmt.Sprintf("Failed to write to MEM cgroup: %s <requestID=%s>", wferr, requestID), 1)
	}

	/* Create a Replica for this Function instance */
	replica := Replica{}
	replica.fname = fname
	replica.ctrType = "wasm"
	replica.uuid = replicaName
	replica.PID = uint32(wasmPid)
	replica.IP = IP
	replica.netNS = netnsNum
	replica.lastAccess = time.Now()
	if setActive {
		fs.AddActiveReplica(&replica)
	} else {
		fs.AddIdleReplica(&replica)
	}
	timec.LogEvent("replicas/createWasmReplica", fmt.Sprintf("Created WASM container for Function '%s' <requestID=%s>", fname, requestID), 2)
	/* Create background thread to wait for runw exit and reap child process */
	go func() { cmd.Wait() }()
	return replica.uuid, replica.IP, nil

	/* TODO: If error starting process, return netnsNum/IP back to pool as available */
}
