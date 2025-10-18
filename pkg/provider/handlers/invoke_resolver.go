package handlers

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/containerd/containerd"
	gocni "github.com/containerd/go-cni"
	fecore "github.gatech.edu/faasedge/fecore/pkg"
	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

const watchdogPort = 8080

type InvokeResolver struct {
	client *containerd.Client
	cni    gocni.CNI
	fs     *FunctionStore
}

func NewInvokeResolver(client *containerd.Client, cni gocni.CNI, fs *FunctionStore) *InvokeResolver {
	return &InvokeResolver{client: client, cni: cni, fs: fs}
}

func (i *InvokeResolver) Resolve(functionName string, requestID string, reqStartupType string, reqContainerType string) (url.URL, string, string, string, error) {
	defer timec.RecordDuration("(invoke_resolver.go) Resolve() <requestID="+requestID+">", time.Now())
	var startupType = ""
	var containerType = ""
	var replicaName = ""
	var replicaIP = ""
	var err error
	actualFunctionName := functionName
	timec.LogEvent("invoke_resolver/Resolve", fmt.Sprintf("New invocation request for %q <requestID=%s>", actualFunctionName, requestID), 2)

	namespace := getNamespace(functionName, fecore.DefaultFunctionNamespace)

	if strings.Contains(functionName, ".") {
		actualFunctionName = strings.TrimSuffix(functionName, "."+namespace)
	}

	function := Function{}
	er := i.fs.GetDeployedFunction(actualFunctionName, &function, requestID)
	if er != nil {
		timec.LogEvent("invoke_resolver/Resolve", fmt.Sprintf("Error retrieving function '%s': %s", actualFunctionName, er), 1)
		return url.URL{}, startupType, containerType, replicaName, er
	}

	if val, ok := function.labels["ctrType"]; ok {
		/* TODO: Should return an error if we fail to resolve container */
		switch val {
		case "native":
			containerType = "native"
			replicaIP, startupType, replicaName, err = i.ResolveNative(&function, requestID, reqStartupType)
			if err != nil {
				timec.LogEvent("invoke_resolver/Resolve", "Unable to resolve Native container type for "+function.name+"<requestID="+requestID+">", 1)
			}
		case "wasm":
			containerType = "wasm"
			replicaIP, startupType, replicaName, err = i.ResolveWasm(&function, requestID, reqStartupType)
			if err != nil {
				timec.LogEvent("invoke_resolver/Resolve", "Unable to resolve WASM container type for "+function.name+"<requestID="+requestID+">", 1)
			}
		case "hybrid":
			containerType = "hybrid"
			replicaIP, startupType, replicaName, err = i.ResolveHybrid(&function, requestID, reqStartupType)
			if err != nil {
				timec.LogEvent("invoke_resolver/Resolve", "Unable to resolve Hybrid container type for "+function.name+"<requestID="+requestID+">", 1)
			}
		default:
			containerType = "native"
			replicaIP, startupType, replicaName, err = i.ResolveNative(&function, requestID, reqStartupType)
			if err != nil {
				timec.LogEvent("invoke_resolver/Resolve", "Unable to resolve default container type (Native) for "+function.name+"<requestID="+requestID+">", 1)
			}
		}
	} else {
		containerType = "native"
		/* Function deployment has no ctrType label; default to native */
		replicaIP, startupType, replicaName, err = i.ResolveNative(&function, requestID, reqStartupType)
		if err != nil {
			timec.LogEvent("invoke_resolver/Resolve", "Unable to resolve default container type (Native) for "+function.name+" <requestID="+requestID+">", 1)
			return url.URL{}, startupType, containerType, replicaName, err
		}
	}

	urlStr := fmt.Sprintf("http://%s:%d", replicaIP, watchdogPort)

	urlRes, err := url.Parse(urlStr)
	if err != nil {
		return url.URL{}, startupType, containerType, replicaName, err
	}

	return *urlRes, startupType, containerType, replicaName, nil
}

func (i *InvokeResolver) ResolveNative(function *Function, requestID string, reqStartupType string) (string, string, string, error) {
	var startupType = ""
	var replicaName = ""
	var replicaIP = ""
	var err error
	/* Try to get a warm container; if successful, return; otherwise, continue to cold start */
	if reqStartupType != "cold" {
		/* Get the next idle replica if there is at least 1 available (warm start); */
		replicaName, replicaIP, err = i.fs.GetIdleReplica(function.name, requestID)
		if err == nil {
			startupType = "warm"
			timec.LogEvent("invoke_resolver/ResolveNative", fmt.Sprintf("Using idle replica '%s' (%s) for Function '%s' <requestID=%s>", replicaName, replicaIP, function.name, requestID), 2)
			return replicaIP, startupType, replicaName, err
		}
	}
	startupType = "cold"
	startTime := time.Now()
	timec.LogEvent("invoke_resolver/ResolveNative", fmt.Sprintf("Creating new replica for Function '%s' <requestID=%s>", function.name, requestID), 2)
	replicaName, replicaIP, err = createReplica(i.fs, i.client, i.cni, function.name, "native", true, requestID)
	if err != nil {
		timec.LogEvent("invoke_resolver/ResolveNative", fmt.Sprintf("Error creating new replica for Function %s: %s", function.name, err), 1)
		return replicaIP, startupType, replicaName, err
	}
	coldStartTime := time.Since(startTime)
	i.fs.RecordColdStartTime(coldStartTime, requestID)
	timec.LogEvent("invoke_resolver/ResolveNative", fmt.Sprintf("Using new replica '%s' (%s) for Function '%s' <requestID=%s>", replicaName, replicaIP, function.name, requestID), 2)
	return replicaIP, startupType, replicaName, err
}

func (i *InvokeResolver) ResolveWasm(function *Function, requestID string, reqStartupType string) (string, string, string, error) {
	var startupType = ""
	var replicaName = ""
	var replicaIP = ""
	var err error
	/* Try to get a warm container; if successful, return; otherwise, continue to cold start */
	if reqStartupType != "cold" {
		/* Get the next idle replica if there is at least 1 available (warm start); */
		replicaName, replicaIP, err = i.fs.GetIdleReplica(function.name, requestID)
		if err == nil {
			startupType = "warm"
			timec.LogEvent("invoke_resolver/ResolveWasm", fmt.Sprintf("Using idle WASM replica '%s' (%s) for Function '%s' <requestID=%s>", replicaName, replicaIP, function.name, requestID), 2)
			return replicaIP, startupType, replicaName, err
		}
	}
	startupType = "cold"
	startTime := time.Now()
	timec.LogEvent("invoke_resolver/ResolveWasm", fmt.Sprintf("Creating new WASM replica for Function '%s' <requestID=%s>", function.name, requestID), 2)
	replicaName, replicaIP, err = createReplica(i.fs, i.client, i.cni, function.name, "wasm", true, requestID)
	if err != nil {
		timec.LogEvent("invoke_resolver/ResolveWasm", fmt.Sprintf("Error creating new WASM replica for Function %s: %s", function.name, err), 1)
		return replicaIP, startupType, replicaName, err
	}
	coldStartTime := time.Since(startTime)
	i.fs.RecordColdStartTime(coldStartTime, requestID)
	timec.LogEvent("invoke_resolver/ResolveWasm", fmt.Sprintf("Using new replica '%s' (%s) for Function '%s' <requestID=%s>", replicaName, replicaIP, function.name, requestID), 2)
	return replicaIP, startupType, replicaName, err
}

func (i *InvokeResolver) ResolveHybrid(function *Function, requestID string, reqStartupType string) (string, string, string, error) {
	var startupType = ""
	var replicaName = ""
	var replicaIP = ""
	var err error
	policy := i.fs.GetInvocationPolicy(function.name)
	/* Check policies to determine what kind of container to use */
	coldStartType := policy.coldStartCtrType
	warmStartType := policy.warmStartCtrType
	warmStartSandbox := function.sandboxes[warmStartType]
	coldStartSandbox := function.sandboxes[coldStartType]
	timec.LogEvent("invoke_resolver/ResolveHybrid", fmt.Sprintf("coldStartType: %s; coldStartSandbox: %s; warmStartType: %s; warmStartSandbox: %s", coldStartType, coldStartSandbox, warmStartType, warmStartSandbox), 3)

	// First see if we have a warm container
	replicaName, replicaIP, err = i.fs.GetIdleReplica(warmStartSandbox, requestID)
	if err == nil {
		startupType = "warm"
		timec.LogEvent("invoke_resolver/ResolveHybrid", fmt.Sprintf("Using idle replica '%s' (%s) for Function '%s' <requestID=%s>", replicaName, replicaIP, function.name, requestID), 2)
		return replicaIP, startupType, replicaName, err
	}
	// If no warm native containers, spawn coldStartCtrType; optionally spawn warmStartCtrType in background, per policy
	startupType = "cold"
	timec.LogEvent("invoke_resolver/ResolveHybrid", fmt.Sprintf("Creating new replica for Function '%s' <requestID=%s>", function.name, requestID), 2)
	replicaName, replicaIP, err = createReplica(i.fs, i.client, i.cni, coldStartSandbox, coldStartType, true, requestID)
	if err != nil {
		timec.LogEvent("invoke_resolver/ResolveHybrid", fmt.Sprintf("Error creating new replica for Function %s: %s", function.name, err), 1)
		return replicaIP, startupType, replicaName, err
	}
	timec.LogEvent("invoke_resolver/ResolveHybrid", fmt.Sprintf("Using new replica '%s' (%s) for Function '%s' <requestID=%s>", replicaName, replicaIP, function.name, requestID), 2)
	// Spawn additional container(s) using a Go func to avoid blocking
	if policy.spawnAddlCtrs > 0 {
		for c := 0; c < policy.spawnAddlCtrs; c++ {
			go func() {
				createReplica(i.fs, i.client, i.cni, function.sandboxes[policy.warmStartCtrType], policy.warmStartCtrType, false, "SPAWN_ADDL")
			}()
		}
	}
	return replicaIP, startupType, replicaName, err
}

func getNamespace(name, defaultNamespace string) string {
	namespace := defaultNamespace
	if strings.Contains(name, ".") {
		namespace = name[strings.LastIndexAny(name, ".")+1:]
	}
	return namespace
}
