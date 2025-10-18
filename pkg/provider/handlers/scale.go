package handlers

import (
	"io/ioutil"
	"log"
	"net/http"

	"github.com/containerd/containerd"
	gocni "github.com/containerd/go-cni"
)

/* TODO: Provide a full implementation for this function. For now, we stub it out since we handle
 * scaling by reusing/creating warm/cold container instances inside invoke_resolver.go
 * This code is stubbed out since faasd's implementation doesn't mesh with our goal of
 * one-container-per-function-instance */
func MakeReplicaUpdateHandler(client *containerd.Client, cni gocni.CNI) func(w http.ResponseWriter, r *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {

		if r.Body == nil {
			http.Error(w, "expected a body", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()

		body, _ := ioutil.ReadAll(r.Body)
		log.Printf("[Scale] request: %s\n", string(body))

		return
	}
}
