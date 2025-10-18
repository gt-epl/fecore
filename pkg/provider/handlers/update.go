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
	"github.com/openfaas/faas-provider/types"

	"github.gatech.edu/faasedge/fecore/pkg/cninetwork"
	"github.gatech.edu/faasedge/fecore/pkg/service"
	"github.gatech.edu/faasedge/fecore/pkg/timec"
)

func MakeUpdateHandler(client *containerd.Client, cni gocni.CNI, secretMountPath string, alwaysPull bool, fs *FunctionStore) func(w http.ResponseWriter, r *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {

		if r.Body == nil {
			http.Error(w, "expected a body", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()

		body, _ := ioutil.ReadAll(r.Body)
		timec.LogEvent("update/MakeUpdateHandler", fmt.Sprintf("New update request: %s", body), 2)

		req := types.FunctionDeployment{}
		err := json.Unmarshal(body, &req)
		if err != nil {
			timec.LogEvent("update/MakeUpdateHandler", fmt.Sprintf("Error parsing update input: %s", err), 1)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := req.Service
		namespace := getRequestNamespace(req.Namespace)

		// Check if namespace exists, and it has the openfaas label
		valid, err := validNamespace(client.NamespaceService(), namespace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if !valid {
			http.Error(w, "namespace not valid", http.StatusBadRequest)
			return
		}

		namespaceSecretMountPath := getNamespaceSecretMountPath(secretMountPath, namespace)

		function := Function{}
		err = fs.GetDeployedFunction(name, &function, "UpdateHandler")
		if err != nil {
			msg := fmt.Sprintf("Cannot update. Service %s not found.", name)
			timec.LogEvent("update/MakeUpdateHandler", msg, 1)
			http.Error(w, msg, http.StatusNotFound)
			return
		}

		err = validateSecrets(namespaceSecretMountPath, req.Secrets)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		ctx := namespaces.WithNamespace(context.Background(), namespace)

		if _, err := prepull(ctx, req, client, alwaysPull); err != nil {
			timec.LogEvent("update/MakeUpdateHandler", fmt.Sprintf("Pre-pull for %s failed: %s", name, err), 1)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		if function.replicas != 0 {
			err = cninetwork.DeleteCNINetwork(ctx, cni, client, name)
			if err != nil {
				timec.LogEvent("update/MakeUpdateHandler", fmt.Sprintf("Error removing CNI network for %s: %s\n", name, err), 1)
			}
		}

		if err := service.Remove(ctx, client, name); err != nil {
			timec.LogEvent("update/MakeUpdateHandler", fmt.Sprintf("Error removing service %s: %s\n", name, err), 1)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// The pull has already been done in prepull, so we can force this pull to "false"
		pull := false

		/* TODO: Replace fn w/ a real function - update handling code is stubbed
		 * out for now. We need to add support for changing already deployed
		 * Functions via this handler */
		fn := Function{}
		if err := deploy(ctx, req, client, cni, namespaceSecretMountPath, pull, &fn, fs, false); err != nil {
			timec.LogEvent("update/MakeUpdateHandler", fmt.Sprintf("Error deploying %s: %s\n", name, err), 1)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
}
