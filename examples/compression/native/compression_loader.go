package main

import (
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"time"
)

func runFn(FnArgs []string) string {
	//FnBinary := "/opt/zpipe"
	FnBinary := "./compression-n"
	cmd := exec.Command(FnBinary, FnArgs...)
	log.Printf("Starting %s", FnBinary)
	result, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal("Unable to run %s: ", FnBinary, err)
	}

	log.Printf("Execution completed. Result = %s", string(result))
	return string(result)
}

func processInvocation(w http.ResponseWriter, r *http.Request) {
	/* Read fn arg(s) from the header(s), add them to a string array */
	var FnArgs []string
	//input_file := r.Header.Get("filename")
    //input_file := "input.file"
	//FnArgs = append(FnArgs, input_file)

	/* Start a timer and run the fn code */
	invocationStart := time.Now()
	fnResult := runFn(FnArgs)
	invocationElapsed := time.Since(invocationStart).Milliseconds()
	execTime := strconv.FormatInt(invocationElapsed, 10)
	log.Printf("Fn completed execution in %s ms", execTime)
	/* Return the exec time in a header */
	w.Header().Set("invocation-elapsed", execTime)

	/* Send a reply with function return in the body */
	io.WriteString(w, fnResult)
}

func main() {
	http.HandleFunc("/", processInvocation)

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Error binding to port 8080")
	}
}
