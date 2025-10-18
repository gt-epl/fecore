package handlers

import (
	"encoding/json"

	"fmt"
	"net/http"
	"strconv"

	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

type MetricsExport struct {
	ColdStart      []int64
	InvocationCold []int64
	InvocationWarm []int64
}

// MakeMetricsHandler creates exposes function/node metrics
func MakeMetricsHandler(fs *FunctionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}

		var jsonOut []byte
		var htmlOut string
		var marshalErr error

		var returnType string

		switch action := r.URL.Query().Get("action"); action {
		case "flush":
			timec.ClearDurationLog()
		case "write":
			timec.WriteDurationLog()
			timec.WriteEventLog("/mnt/faasedge/logs/fecore-events.log")
		case "metrics":
			returnType = "json"
			fname := r.URL.Query().Get("fname")
			fstat := GetMetricsLog(fs, fname)
			jsonOut, marshalErr = json.Marshal(fstat)
		case "stats":
			returnType = "html"
			fname := r.URL.Query().Get("fname")
			htmlOut = GetMetricsReport(fs, fname)
		default:
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

func GetMetricsLog(fs *FunctionStore, fname string) [100]FunctionStat {
	fstat := fs.functionStats[fname].Entries
	return fstat
}

func GetMetricsReport(fs *FunctionStore, fname string) string {

	fs.functionStats[fname].statMu.RLock()
	totalInvocations := strconv.FormatInt(fs.functionStats[fname].totalInvocations, 10)
	numStats := strconv.Itoa(fs.functionStats[fname].entryPos)
	var totalSetupTime int
	var totalExecTime int
	for _, startupTime := range fs.functionStats[fname].startupTimes {
		totalSetupTime += startupTime
	}
	for _, execTime := range fs.functionStats[fname].execTimes {
		totalExecTime += execTime
	}

	avgExecTime := fmt.Sprintf("%d", fs.functionStats[fname].avgExecTime)
	avgStartupTime := fmt.Sprintf("%d", fs.functionStats[fname].avgStartupTime)
	var avgServiceTime string
	if fs.functionStats[fname].totalInvocations > 0 {
		avgServiceTime = "0"
	} else {
		avgServiceTime = fmt.Sprintf("%d", (fs.functionStats[fname].totalExecTime+fs.functionStats[fname].totalStartupTime)/fs.functionStats[fname].totalInvocations)
	}
	p50ServiceTime := fmt.Sprintf("%d", fs.functionStats[fname].p50SvcTime)
	p99ServiceTime := fmt.Sprintf("%d", fs.functionStats[fname].p99SvcTime)
	sandboxUtilization := fmt.Sprintf("%.2f", fs.functionStats[fname].sandboxUtil)
	avgSvcCold := fmt.Sprintf("%d", fs.functionStats[fname].avgSvcCold)
	avgSvcWarm := fmt.Sprintf("%d", fs.functionStats[fname].avgSvcWarm)
	fs.functionStats[fname].statMu.RUnlock()

	fs.deployedFunctions[fname].policyMu.Lock()
	policy := fs.deployedFunctions[fname].policy
	coldStartCtrType := policy.coldStartCtrType
	warmStartCtrType := policy.warmStartCtrType
	spawnAddlCtr := fmt.Sprintf("%d", policy.spawnAddlCtrs)
	/* Update the policy */
	fs.deployedFunctions[fname].policyMu.Unlock()

	reportHeader := "<!DOCTYPE html><html><head><title>" + fname + `</title>
	<style>
table.replicas {
  font-family: arial, sans-serif;
  border-collapse: collapse;
  width: 100%;
	td, th {
		border: 1px solid #dddddd;
		text-align: left;
		padding: 8px;
	}
	
	tr:nth-child(even) {
		background-color: #dddddd;
	}
}

table.stats {
	border: 0px solid
	width: 30%;
	td, th {
		border: 0px solid;
		text-align: right;
		padding: 1px;
	}
	tr {
		background-color #000000;
	}
}
</style></head><body>
<h1>` + fname + `</h1>
<hr>
<h2>Stats</h2>
<table border=0, class="stats">
<tr><td align="right">Num Stats: </td><td>` + numStats + `</td></tr>
<tr><td align="right">Avg. Exec Time: </td><td>` + avgExecTime + `</td></tr>
<tr><td align="right">Avg. Startup Time: </td><td>` + avgStartupTime + `</td></tr>
<tr><td align="right">Avg. Service Time: </td><td>` + avgServiceTime + `</td></tr>
<tr><td align="right">P50 Service Time: </td><td>` + p50ServiceTime + `</td></tr>
<tr><td align="right">P99 Service Time: </td><td>` + p99ServiceTime + `</td></tr>
<tr><td align="right">Total Invocations: </td><td>` + totalInvocations + `</td></tr>
<tr><td align="right">Sandbox Utilization: </td><td>` + sandboxUtilization + `</td></tr>
<tr><td>Avg. Svc. Cold: </td><td>` + avgSvcCold + `</td></tr>
<tr><td>Avg. Svc. Warm: </td><td>` + avgSvcWarm + `</td></tr>
</table>
<hr>
<h2>Policy</h2>
<table border=0, class="stats">
<tr><td>Cold Start Sandbox: </td><td>` + coldStartCtrType + `</td></tr>
<tr><td>Warm Start Sandbox: </td><td>` + warmStartCtrType + `</td></tr>
<tr><td>Spawn Addl Sandbox: </td><td>` + spawnAddlCtr + `</td></tr>
</table>
<hr>`
	reportFooter := "</body></html>"
	nativeReplicaCount := 0
	wasmReplicaCount := 0
	fn := fs.deployedFunctions[fname]
	/* Get idle replicas */
	reportIdleReplicas := `<br><p><h2>Idle Replicas</h2><table class="replicas"><tr><th>UUID</th><th>PID</th><th>IP</th><th>Type</th></tr>`

	if fn.idleReplicas.LRU != nil {
		replica := fn.idleReplicas.LRU
		reportIdleReplicas += "<tr><td>" + replica.uuid + " (LRU) </td><td>" + strconv.FormatInt(int64(replica.PID), 10) + "</td><td>" + replica.IP + "</td><td>" + replica.ctrType + "</td></tr>"
	}

	if fn.idleReplicas.MRU != nil {
		replica := fn.idleReplicas.MRU
		reportIdleReplicas += "<tr><td>" + replica.uuid + " (MRU) </td><td>" + strconv.FormatInt(int64(replica.PID), 10) + "</td><td>" + replica.IP + "</td><td>" + replica.ctrType + "</td></tr>"
		for _, replica = range fn.idleReplicas.containers {
			if replica.ctrType == "native" {
				nativeReplicaCount += 1
			}
			if replica.ctrType == "wasm" {
				wasmReplicaCount += 1
			}
			reportIdleReplicas += "<tr><td>" + replica.uuid + "</td><td>" + strconv.FormatInt(int64(replica.PID), 10) + "</td><td>" + replica.IP + "</td><td>" + replica.ctrType + "</td></tr>"
		}
		reportIdleReplicas += "</table>"
	}

	/* Get active replicas */
	reportActiveReplicas := `<br><p><h2>Active Replicas</h2><table class="replicas"><tr><th>UUID</th><th>PID</th><th>IP</th><th>Type</th></tr>`
	for _, replica := range fn.activeReplicas {
		if replica.ctrType == "native" {
			nativeReplicaCount += 1
		}
		if replica.ctrType == "wasm" {
			wasmReplicaCount += 1
		}
		reportActiveReplicas += "<tr><td>" + replica.uuid + "</td><td>" + strconv.FormatInt(int64(replica.PID), 10) + "</td><td>" + replica.IP + "</td><td>" + replica.ctrType + "</td></tr>"
	}
	reportActiveReplicas += "</table>"

	return reportHeader + reportActiveReplicas + reportIdleReplicas + reportFooter
}
