package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	gocni "github.com/containerd/go-cni"
	"github.com/openfaas/faas/gateway/requests"
	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

func MakeDeleteHandler(client *containerd.Client, cni gocni.CNI, fs *FunctionStore) func(w http.ResponseWriter, r *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {

		if r.Body == nil {
			http.Error(w, "expected a body", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()

		body, _ := ioutil.ReadAll(r.Body)
		timec.LogEvent("delete/MakeDeleteHandler", fmt.Sprintf("Delete requested for %s", string(body)), 2)

		req := requests.DeleteFunctionRequest{}
		err := json.Unmarshal(body, &req)
		if err != nil {
			timec.LogEvent("delete/MakeDeleteHandler", fmt.Sprintf("Error parsing input: %s", err), 1)
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		lookupNamespace := getRequestNamespace(readNamespaceFromQuery(r))

		// Check if namespace exists, and it has the openfaas label
		valid, err := validNamespace(client.NamespaceService(), lookupNamespace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if !valid {
			http.Error(w, "namespace not valid", http.StatusBadRequest)
			return
		}

		name := req.FunctionName

		ctx := namespaces.WithNamespace(context.Background(), lookupNamespace)

		// Remove each Function replica
		fs.DeleteAllReplicas(ctx, client, cni, name)
		fs.RemoveDeployedFunction(name)

		timec.LogEvent("delete/MakeDeleteHandler", fmt.Sprintf("Deleted function '%s'", name), 2)
	}
}
