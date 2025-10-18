# Developing fecore

This doc provides quick overview of how to navigate fecore's codebase.
<br>

Server setup begins in `cmd/provider.go`. In this file we load fecore's config, initialize data structures needed to keep track of Function metadata, and declare handlers for API endpoints.

Most API actions are tied to handlers, which are pieces of Golang code that handle operations such as deploying new Functions and servicing Function invocation requests. These definitions can be found in the `bootstrapHandlers` var in `cmd/provider.go`.
<br>

Handler definitions are in the `pkg/provider/handlers` directory. The following files correspond to handlers:
- `delete.go` handles the deletion of a deployed Function
- `deploy.go` handles new Function deployments
- `info.go` reports basic info about fecore, such as version number and orchestration used
- `invoke_resolver.go` handles Function invocation requests
- `metrics.go` handles access to metrics for deployed Functions
- `namespaces.go` lists Function namespaces. This feature is currently unused in fecore, but allows Functions to be grouped by namespace, which is necessary for a more robust multi-tenant setup.
- `read.go` handles requests to list currently deployed Functions
- `replicas.go` handles the creation of Function replicas. The `invoke_resolver` relies heavily on this code.
- `scale.go` handles scaling of Function replcias. This feature is currently unused in fecore.
- `secret.go` handles management of a container's shared secret files. This feature is currently unused in fecore.
- `update.go` handles updating metadata for a deployed Function.
<br>

Other files of interest in the `pkg/provider/handlers` directory:
- `wasm_network.go` contains code for creating virtual network interfaces for use with WASM containers.
- `utils.go` contains code for basic helper operations.
- `stats.go` contains code for gathering statistics on deployed Functions.
- `policy.go` contains code for managing policy related to deployed Functions.
- `ipam.go` contains code for IP address management of the container network. This code is currently unused.
- `function_store.go` contains code for the Function store (see below section for more info).
- `functions.go` contains definitions for the data structures that hold metadata for deployed Functions and their Replicas.
- `proxy/function_proxy.go` contains code that proxies Function invocation requests from clients to the fecore server.

## The Function Store
fecore holds metadata for Functions and their Replicas in an in-memory object known as the Function Store. The Function Store is instantiated upon fecore startup in `cmd/provider.go`. The `pkg/provider/handlers/function_store.go` file containers helper code to manage the Function store.
<br>

It is important to note that the Function Store makes extensive use of mutexes throughout since multiple threads may simultaneously need to read/write Function metadata. If you plan on doing anything with Function metadata, you should take great care to ensure that (1) there are minimal interfaces to read/write that metadata and (2) mutexes are used to protect access to that metadata.

