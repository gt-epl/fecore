package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

type Policy struct {
	coldStartCtrType      string
	warmStartCtrType      string
	spawnAddlCtrs         int
	keepaliveColdStartCtr int
}

type policyJSON struct {
	ColdStartCtrType      string `json:"coldStartCtrType"`
	WarmStartCtrType      string `json:"warmStartCtrType"`
	SpawnAddlCtrs         int    `json:"spawnAddlCtrs"`
	KeepaliveColdStartCtr int    `json:"keepaliveColdStartCtr"`
}

/* Handles Policy API endpoint */
func MakePolicyHandler(fs *FunctionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}

		var jsonOut []byte
		var htmlOut string
		var marshalErr error

		var returnType string

		switch action := r.URL.Query().Get("action"); action {
		case "view":
			returnType = "json"
			fname := r.URL.Query().Get("fname")
			timec.LogEvent("MakePolicyHandler", fmt.Sprintf("Got view request for %s", fname), 3)
			policy := GetPolicyView(fs, fname)
			jsonOut, marshalErr = json.Marshal(policy)
		case "update":
			var updatedPolicy Policy
			returnType = "json"
			fname := r.URL.Query().Get("fname")
			if v := r.URL.Query().Get("coldStartCtrType"); v != "" {
				updatedPolicy.coldStartCtrType = v
			} else {
				updatedPolicy.coldStartCtrType = ""
			}
			if v := r.URL.Query().Get("warmStartCtrType"); v != "" {
				updatedPolicy.warmStartCtrType = v
			} else {
				updatedPolicy.warmStartCtrType = ""
			}
			if v := r.URL.Query().Get("spawnAddlCtrs"); v != "" {
				val, err := strconv.Atoi(v)
				if err != nil {
					val = -1
				}
				updatedPolicy.spawnAddlCtrs = val
			} else {
				updatedPolicy.spawnAddlCtrs = -1
			}
			if v := r.URL.Query().Get("keepaliveColdStartCtr"); v != "" {
				val, err := strconv.Atoi(v)
				if err != nil {
					val = -1
				}
				updatedPolicy.keepaliveColdStartCtr = val
			} else {
				updatedPolicy.keepaliveColdStartCtr = -1
			}
			policy := UpdatePolicy(fs, fname, updatedPolicy)
			jsonOut, marshalErr = json.Marshal(policy)
		default:
			// TODO: Should return default policy as JSON
			returnType = "json"
			timec.WriteDurationLog()
			jsonOut, marshalErr = json.Marshal(timec.TimeLog)
		}

		if marshalErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if returnType == "json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(jsonOut)
		} else {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(htmlOut))
		}
	}
}

func GetPolicyView(fs *FunctionStore, fn string) policyJSON {
	defer fs.deployedFunctions[fn].policyMu.RUnlock()
	fs.deployedFunctions[fn].policyMu.RLock()
	policy := fs.deployedFunctions[fn].policy
	view := policyJSON{
		ColdStartCtrType:      policy.coldStartCtrType,
		WarmStartCtrType:      policy.warmStartCtrType,
		SpawnAddlCtrs:         policy.spawnAddlCtrs,
		KeepaliveColdStartCtr: policy.keepaliveColdStartCtr,
	}
	timec.LogEvent("GetPolicy", fmt.Sprintf("Got policy for %s", fn), 3)
	return view
}

func UpdatePolicy(fs *FunctionStore, fn string, updatedPolicy Policy) policyJSON {
	defer fs.deployedFunctions[fn].policyMu.RUnlock()
	fs.deployedFunctions[fn].policyMu.RLock()
	// var jsonOut []byte
	// var marshalErr error
	// TODO: Should add a helper function to validate ctrTypes based on what the platform accepts
	if updatedPolicy.coldStartCtrType == "native" || updatedPolicy.coldStartCtrType == "wasm" {
		fs.deployedFunctions[fn].policy.coldStartCtrType = updatedPolicy.coldStartCtrType
		timec.LogEvent("UpdatedPolicy", fmt.Sprintf("Changed coldStartCtrType to %s for %s", updatedPolicy.coldStartCtrType, fn), 3)
	}

	if updatedPolicy.warmStartCtrType == "native" || updatedPolicy.warmStartCtrType == "wasm" {
		fs.deployedFunctions[fn].policy.warmStartCtrType = updatedPolicy.warmStartCtrType
		timec.LogEvent("UpdatedPolicy", fmt.Sprintf("Changed warmStartCtrType to %s for %s", updatedPolicy.warmStartCtrType, fn), 3)
	}

	if updatedPolicy.spawnAddlCtrs != -1 && updatedPolicy.spawnAddlCtrs < fs.MAX_ADDL_CTRS {
		fs.deployedFunctions[fn].policy.spawnAddlCtrs = updatedPolicy.spawnAddlCtrs
	}

	if updatedPolicy.keepaliveColdStartCtr >= 0 && updatedPolicy.keepaliveColdStartCtr < fs.MAX_KEEPALIVE_TIME {
		fs.deployedFunctions[fn].policy.keepaliveColdStartCtr = updatedPolicy.keepaliveColdStartCtr
	}

	currentPolicy := fs.deployedFunctions[fn].policy
	view := policyJSON{
		ColdStartCtrType:      currentPolicy.coldStartCtrType,
		WarmStartCtrType:      currentPolicy.warmStartCtrType,
		SpawnAddlCtrs:         currentPolicy.spawnAddlCtrs,
		KeepaliveColdStartCtr: currentPolicy.keepaliveColdStartCtr,
	}
	return view
}

func (fs *FunctionStore) EvalSpawnAddlCtrs(fn string, utilizationRatio float32, coldRatio float32) {
	timec.LogEvent("policy/EvalSpawnAddlCtrs", fmt.Sprintf("Cold ratio for %s is %.2f", fn, coldRatio), 5)
	var COLD_RATIO_LOW float32
	var COLD_RATIO_HIGH float32
	var spawnAddlCtrs int
	/* Thresholds to use when considering percentage of requests in the
	 * last epoch that were served from cold start */
	COLD_RATIO_LOW = 0.10
	COLD_RATIO_HIGH = 0.25

	spawnAddlCtrs = 0

	/* Cold starts are low; decrease spawnAddlCtrs */
	if coldRatio <= COLD_RATIO_LOW {
		spawnAddlCtrs = -1
		/* Lots of cold starts; increase spawnAddlCtrs */
	} else if coldRatio >= COLD_RATIO_HIGH {
		spawnAddlCtrs = 1
	}

	timec.LogEvent("policy/EvalSpawnAddlCtrs", fmt.Sprintf("Updating %s policy.SpawnAddlCtrs by %d", fn, spawnAddlCtrs), 4)
	fs.deployedFunctions[fn].policyMu.Lock()
	fs.deployedFunctions[fn].policy.spawnAddlCtrs += spawnAddlCtrs
	fs.deployedFunctions[fn].policyMu.Unlock()
}

func (fs *FunctionStore) EvalSandboxUtilization(fn string) {
	var nativeDeployment string
	var wasmDeployment string
	fs.dfMu.RLock()
	if fs.deployedFunctions[fn].labels["ctrType"] == "hybrid" {
		nativeDeployment = fs.deployedFunctions[fn].sandboxes["native"]
		wasmDeployment = fs.deployedFunctions[fn].sandboxes["wasm"]
	} else {
		timec.LogEvent("function_store/EvalSandboxUtilization", fmt.Sprintf("Function %s is not hybrid. Cannot eval sandbox utilization.", fn), 3)
	}
	fs.dfMu.RUnlock()

	var nativeIdleCount int
	var nativeActiveCount int
	var wasmIdleCount int
	var wasmActiveCount int
	var coldRatio float32

	fs.deployedFunctions[nativeDeployment].activeReplicasLock.RLock()
	nativeActiveCount = len(fs.deployedFunctions[nativeDeployment].activeReplicas)
	fs.deployedFunctions[nativeDeployment].activeReplicasLock.RUnlock()
	fs.deployedFunctions[nativeDeployment].idleReplicasLock.RLock()
	nativeIdleCount = int(fs.deployedFunctions[nativeDeployment].idleReplicas.count)
	fs.deployedFunctions[nativeDeployment].idleReplicasLock.RUnlock()

	fs.deployedFunctions[wasmDeployment].activeReplicasLock.RLock()
	wasmActiveCount = len(fs.deployedFunctions[wasmDeployment].activeReplicas)
	fs.deployedFunctions[wasmDeployment].activeReplicasLock.RUnlock()
	fs.deployedFunctions[wasmDeployment].idleReplicasLock.RLock()
	wasmIdleCount = int(fs.deployedFunctions[wasmDeployment].idleReplicas.count)
	fs.deployedFunctions[wasmDeployment].idleReplicasLock.RUnlock()

	totalNative := nativeIdleCount + nativeActiveCount
	totalWasm := wasmIdleCount + wasmActiveCount
	utilizationRatio := float32(nativeActiveCount+wasmActiveCount) / float32(totalNative+totalWasm)

	fs.functionStats[fn].statMu.Lock()
	fs.functionStats[fn].sandboxUtil = utilizationRatio
	fs.functionStats[fn].activeCount = nativeActiveCount + wasmActiveCount
	fs.functionStats[fn].idleCount = nativeIdleCount + wasmIdleCount
	coldRatio = fs.functionStats[fn].coldRatio
	fs.functionStats[fn].statMu.Unlock()

	fs.EvalSpawnAddlCtrs(fn, utilizationRatio, coldRatio)

	timec.LogEvent("policy/EvalSandboxUtilization", fmt.Sprintf("%s has sandbox utilization ratio of %.2f (NATIVE: %d/%d; WASM: %d/%d)", fn, utilizationRatio, nativeActiveCount, nativeIdleCount, wasmActiveCount, wasmIdleCount), 4)
}

func (fs *FunctionStore) EvalColdStartPolicy(fn string) {
	var nativeDeployment string
	var wasmDeployment string
	fs.dfMu.RLock()
	if fs.deployedFunctions[fn].labels["ctrType"] == "hybrid" {
		nativeDeployment = fs.deployedFunctions[fn].sandboxes["native"]
		wasmDeployment = fs.deployedFunctions[fn].sandboxes["wasm"]
	} else {
		timec.LogEvent("function_store/EvalColdStartPolicy", fmt.Sprintf("Function %s is not hybrid. Cannot eval cold start policy.", fn), 3)
	}
	fs.dfMu.RUnlock()

	/* Retrieve stats for Hybrid's Native and WASM sandboxes */
	fs.functionStats[nativeDeployment].statMu.RLock()
	native_avgSvcCold := fs.functionStats[nativeDeployment].avgSvcCold
	fs.functionStats[nativeDeployment].statMu.RUnlock()

	fs.functionStats[wasmDeployment].statMu.RLock()
	wasm_avgSvcCold := fs.functionStats[wasmDeployment].avgSvcCold
	fs.functionStats[wasmDeployment].statMu.RUnlock()
	/* Compare stats to determine the policy */
	var coldStartCtrType string
	var spawnAddlCtrs int

	timec.LogEvent("====> EvalColdStartPolicy", fmt.Sprintf("native_avgSvcCold: %d vs. wasm_avgSvcCold: %d", native_avgSvcCold, wasm_avgSvcCold), 4)
	/* For cold starts, we want lowest avg service time */
	if native_avgSvcCold < wasm_avgSvcCold {
		coldStartCtrType = "native"
		spawnAddlCtrs = 0
	} else {
		coldStartCtrType = "wasm"
		spawnAddlCtrs = 1
	}

	/* Update the policy */
	fs.deployedFunctions[fn].policyMu.Lock()
	fs.deployedFunctions[fn].policy.coldStartCtrType = coldStartCtrType
	if coldStartCtrType == fs.deployedFunctions[fn].policy.warmStartCtrType {
		fs.deployedFunctions[fn].policy.keepaliveColdStartCtr = 60
	}
	fs.deployedFunctions[fn].policy.spawnAddlCtrs = spawnAddlCtrs
	fs.deployedFunctions[fn].policyMu.Unlock()
}

func (fs *FunctionStore) EvalWarmStartPolicy(fn string) {
	var nativeDeployment string
	var wasmDeployment string
	fs.dfMu.RLock()
	if fs.deployedFunctions[fn].labels["ctrType"] == "hybrid" {
		nativeDeployment = fs.deployedFunctions[fn].sandboxes["native"]
		wasmDeployment = fs.deployedFunctions[fn].sandboxes["wasm"]
	} else {
		timec.LogEvent("function_store/EvalColdStartPolicy", fmt.Sprintf("Function %s is not hybrid. Cannot eval warm start policy.", fn), 3)
	}
	fs.dfMu.RUnlock()

	/* Retrieve stats for Hybrid's Native and WASM sandboxes */
	fs.functionStats[nativeDeployment].statMu.RLock()
	native_avgSvcWarm := fs.functionStats[nativeDeployment].avgSvcWarm
	fs.functionStats[nativeDeployment].statMu.RUnlock()

	fs.functionStats[wasmDeployment].statMu.RLock()
	wasm_avgSvcWarm := fs.functionStats[wasmDeployment].avgSvcWarm
	fs.functionStats[wasmDeployment].statMu.RUnlock()
	/* Compare stats to determine the policy */
	var warmStartCtrType string

	timec.LogEvent("====> EvalWarmStartPolicy", fmt.Sprintf("native_avgSvcWarm: %d vs. wasm_avgSvcWarm: %d", native_avgSvcWarm, wasm_avgSvcWarm), 4)
	/* For warm starts, we want lowest avg service time */
	if native_avgSvcWarm < wasm_avgSvcWarm {
		warmStartCtrType = "native"
	} else {
		warmStartCtrType = "wasm"
	}
	/* Update the policy */
	fs.deployedFunctions[fn].policyMu.Lock()
	fs.deployedFunctions[fn].policy.warmStartCtrType = warmStartCtrType
	fs.deployedFunctions[fn].policyMu.Unlock()
}

func (fs *FunctionStore) GetInvocationPolicy(fn string) Policy {
	defer fs.deployedFunctions[fn].policyMu.RUnlock()
	fs.deployedFunctions[fn].policyMu.RLock()
	policy := fs.deployedFunctions[fn].policy
	tmp := Policy{}
	tmp.coldStartCtrType = policy.coldStartCtrType
	tmp.warmStartCtrType = policy.warmStartCtrType
	tmp.spawnAddlCtrs = policy.spawnAddlCtrs
	tmp.keepaliveColdStartCtr = policy.keepaliveColdStartCtr
	return tmp
}
