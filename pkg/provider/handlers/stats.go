package handlers

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

type FunctionStats struct {
	Entries          [100]FunctionStat
	entryPos         int
	coldPos          int
	warmPos          int
	coldStarts       int // number cold starts in current epoch
	warmStarts       int // number warm starts in current epoch
	currRPS          int // average RPS over a 10 second window
	lastRPS          int // previous average RPS recorded
	currInvocations  int // number invocations in current epoch
	invokeNext       string
	activeCount      int // number of active replicas
	idleCount        int // number of idle replicas
	totalInvocations int64
	totalExecTime    int64
	totalStartupTime int64
	totalSvcTime     int
	totalSvcCold     int
	totalSvcWarm     int
	avgExecTime      int64
	avgStartupTime   int64
	p99SvcTime       int
	p50SvcTime       int
	avgSvcTime       int
	avgSvcCold       int
	avgSvcWarm       int
	sandboxUtil      float32
	coldRatio        float32
	warmRatio        float32
	// epochTime        time.Time // time when last epoch was reached
	execTimes    [100]int
	startupTimes [100]int
	serviceTimes [100]int
	statMu       sync.RWMutex
}

type FunctionStat struct {
	Fn          string
	CtrType     string
	StartupTime int64
	ExecTime    int64
	StartupType string
}

func (fs *FunctionStore) UpdateFunctionStats(fn string, ctrType string, requestID string, startupTime int64, execTime int64, startupType string) {
	timec.LogEvent("function_store/UpdateFunctionStats", fmt.Sprintf("Updating Function stats for '%s' (ctrType=%s, startupTime=%d, execTime=%d) <requestID=%s>", fn, ctrType, startupTime, execTime, requestID), 2)

	/* If hybrid, add two entries: one to the stats of the hybrid container and
	 * a duplicate entry to the stats of the container type that actually serviced the invocation */
	if ctrType == "hybrid" {
		tokens := strings.Split(requestID, "_")
		sandboxName := tokens[0]
		sandboxType := tokens[2]
		if sandboxType == "n" {
			fs.statsChan <- FunctionStat{fn, "native", startupTime, execTime, startupType}
			fs.statsChan <- FunctionStat{sandboxName, "native", startupTime, execTime, startupType}
		} else if sandboxType == "w" {
			fs.statsChan <- FunctionStat{fn, "wasm", startupTime, execTime, startupType}
			fs.statsChan <- FunctionStat{sandboxName, "wasm", startupTime, execTime, startupType}
		}
	} else {
		// Add stats to the deployed container type
		fs.statsChan <- FunctionStat{fn, ctrType, startupTime, execTime, startupType}
	}
}

func (fs *FunctionStore) ProcessFunctionStats() {
	/* Use circular buffer for stats
	 * Based on idea from:
	 * https://stackoverflow.com/questions/55598220/efficiently-keeping-a-collection-of-the-last-n-pushed-items */
	MAX_ENTRIES := 100
	for {
		select {
		case stat := <-fs.statsChan:
			fn := stat.Fn
			_, ok := fs.functionStats[fn]
			if !ok {
				timec.LogEvent("stats/ProcessFunctionStats", fmt.Sprintf("ERROR: Unable to add stat: could not find %s in functionStats map", fn), 1)
				continue
			}
			fs.functionStats[fn].statMu.Lock()
			entryPos := fs.functionStats[fn].entryPos
			coldPos := fs.functionStats[fn].coldPos
			warmPos := fs.functionStats[fn].warmPos
			fs.functionStats[fn].currInvocations += 1
			/* Every 100 stats, calculate P50 and P99 */
			if entryPos == 99 {
				sort.Ints(fs.functionStats[fn].serviceTimes[:])
				fs.functionStats[fn].p50SvcTime = fs.functionStats[fn].serviceTimes[49]
				fs.functionStats[fn].p99SvcTime = fs.functionStats[fn].serviceTimes[98]
				/* Reset epoch stats */
				fs.functionStats[fn].totalSvcTime = 0
				timec.LogEvent("function_store/ProcessFunctionStats", fmt.Sprintf("Got 100 entries for %s; p50 = %d; p99 = %d", fn, fs.functionStats[fn].p50SvcTime, fs.functionStats[fn].p99SvcTime), 3)
			}
			if coldPos == 99 {
				fs.functionStats[fn].totalSvcCold = 0
			}
			if warmPos == 99 {
				fs.functionStats[fn].totalSvcWarm = 0
			}
			fs.functionStats[fn].totalSvcTime += int(stat.StartupTime) + int(stat.ExecTime)
			fs.functionStats[fn].avgSvcTime = fs.functionStats[fn].totalSvcTime / (entryPos + 1)
			fs.functionStats[fn].Entries[entryPos] = stat
			// fs.functionStats[fn].entryPos = (entryPos + 1) % MAX_ENTRIES
			fs.functionStats[fn].totalInvocations += 1
			fs.functionStats[fn].execTimes[entryPos] = int(stat.ExecTime)
			fs.functionStats[fn].startupTimes[entryPos] = int(stat.StartupTime)
			fs.functionStats[fn].serviceTimes[entryPos] = int(stat.ExecTime) + int(stat.StartupTime)
			fs.functionStats[fn].totalExecTime += stat.ExecTime
			fs.functionStats[fn].totalStartupTime += stat.StartupTime
			fs.functionStats[fn].avgExecTime = (fs.functionStats[fn].totalExecTime / fs.functionStats[fn].totalInvocations)
			fs.functionStats[fn].avgStartupTime = (fs.functionStats[fn].totalStartupTime / fs.functionStats[fn].totalInvocations)
			if stat.StartupType == "cold" {
				fs.functionStats[fn].coldStarts += 1
				fs.functionStats[fn].totalSvcCold = int(stat.ExecTime) + int(stat.StartupTime)
				fs.functionStats[fn].avgSvcCold = (fs.functionStats[fn].totalSvcCold / (coldPos + 1))
				fs.functionStats[fn].coldPos = (coldPos + 1) % MAX_ENTRIES
			} else if stat.StartupType == "warm" {
				fs.functionStats[fn].warmStarts += 1
				fs.functionStats[fn].totalSvcWarm = int(stat.ExecTime) + int(stat.StartupTime)
				fs.functionStats[fn].avgSvcWarm = (fs.functionStats[fn].totalSvcWarm / (warmPos + 1))
				fs.functionStats[fn].warmPos = (warmPos + 1) % MAX_ENTRIES
			}
			if stat.CtrType == "hybrid" {
				if fs.functionStats[fn].currInvocations == fs.cfg.InvocationSampleThreshold {
					go func() {
						fs.EvalSandboxUtilization(fn)
					}()
					coldRatio := float32(fs.functionStats[fn].coldStarts) / float32(fs.cfg.InvocationSampleThreshold)
					warmRatio := float32(fs.functionStats[fn].warmStarts) / float32(fs.cfg.InvocationSampleThreshold)
					fs.functionStats[fn].coldRatio = coldRatio
					fs.functionStats[fn].warmRatio = warmRatio
					timec.LogEvent("stats/ProcessFunctionStats", fmt.Sprintf("====> EVAL WARM/COLD RATIO for %s: warm=%f ; cold=%f", fn, warmRatio, coldRatio), 4)
					fs.functionStats[fn].currInvocations = 0
					fs.functionStats[fn].warmStarts = 0
					fs.functionStats[fn].coldStarts = 0
				}
				if (coldPos+1)%10 == 0 {
					go func() {
						fs.EvalColdStartPolicy(fn)
					}()
				}
				if (warmPos+1)%10 == 0 {
					go func() {
						fs.EvalWarmStartPolicy(fn)
					}()
				}
			}
			fs.functionStats[fn].entryPos = (entryPos + 1) % MAX_ENTRIES
			fs.functionStats[fn].statMu.Unlock()
		}
	}
}

func (fs *FunctionStore) RecordColdStartTime(csTime time.Duration, requestID string) {
	defer timec.RecordDuration("(function_store.go) RecordColdStartTime() <requestID="+requestID+">", time.Now())
	fs.metricMu.Lock()
	defer fs.metricMu.Unlock()

	fs.coldStartTimes = append(fs.coldStartTimes, csTime.Milliseconds())
}

func (fs *FunctionStore) GetColdStartTimes() []int64 {
	fs.metricMu.RLock()
	defer fs.metricMu.RUnlock()

	return fs.coldStartTimes
}

func (fs *FunctionStore) RecordInvocationTime(invocationTime int64, startupType string) {
	fs.metricMu.Lock()
	defer fs.metricMu.Unlock()

	if startupType == "cold" {
		fs.invocationTimesCold = append(fs.invocationTimesCold, invocationTime)
	} else {
		fs.invocationTimesWarm = append(fs.invocationTimesWarm, invocationTime)
	}
}

func (fs *FunctionStore) GetInvocationTime(cold bool) []int64 {
	fs.metricMu.RLock()
	defer fs.metricMu.RUnlock()

	if cold {
		return fs.invocationTimesCold
	} else {
		return fs.invocationTimesWarm
	}
}
