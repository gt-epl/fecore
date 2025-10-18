package timec

import (
	"fmt"
	"log"
	"os"
	"time"
)

type TimeTrack struct {
	Fn       string
	Duration int64
}

var TimeLog []TimeTrack

var EventLog []string

func ClearDurationLog() {
	TimeLog = TimeLog[:0]
}

func WriteDurationLog() {
	filename := fmt.Sprintf("/mnt/faasedge/logs/timec-%d.log", time.Now().UnixMilli())
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}
	defer f.Close()
	// Disable timestamp
	//log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
	log.SetFlags(0)
	log.SetOutput(f)
	for _, v := range TimeLog {
		log.Printf("%s, %d\n", v.Fn, v.Duration)
	}
}

func WriteEventLog(output ...string) {
	var filename string
	if len(output) > 0 {
		filename = output[0]
	} else {
		filename = fmt.Sprintf("/mnt/faasedge/logs/fecore-events-%d.log", time.Now().UnixMilli())
	}
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening fecore events log file: %v", err)
	}
	defer f.Close()
	for _, v := range EventLog {
		f.WriteString(v + "\n")
	}
}

func RecordDuration(fn string, eventStart time.Time) {
	duration := time.Since(eventStart)
	event := TimeTrack{}
	event.Fn = fn
	event.Duration = duration.Microseconds()
	// fmt.
	TimeLog = append(TimeLog, event)
}

func LogEvent(tag string, msg string, console ...int) {
	/* Log levels:
	 * 1 = Critical
	 * 2 = Info
	 * 3 = Debug
	 * 4 = Reserved
	 */
	DEFAULT_LOG_LEVEL := 2 // TODO: Put this in config
	CURR_LOG_LEVEL := 1    // TOOD: Put this in config
	currTime := time.Now()
	timestamp := fmt.Sprintf("%d/%d/%d %02d:%02d:%02d:%02d", currTime.Year(), currTime.Month(), currTime.Day(),
		currTime.Hour(), currTime.Minute(), currTime.Second(), currTime.Nanosecond())
	event := "<" + timestamp + "> " + "[" + tag + "] " + msg
	EventLog = append(EventLog, event)

	var msgLogLevel int
	if len(console) > 0 {
		msgLogLevel = console[0]
	} else {
		msgLogLevel = DEFAULT_LOG_LEVEL
	}

	// log.Printf("[%s] %s\n", tag, msg) // Force print to console
	if msgLogLevel >= CURR_LOG_LEVEL {
		log.Printf("[%s] %s\n", tag, msg)
	}
}
