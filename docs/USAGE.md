# Using fecore

The `faas-cli` tool from OpenFaaS serves as a front-end to fecore.
Alternatively, fecore can be managed directly by API calls via HTTP endpoints.

fecore supports three function types:
1. Traditional or native process-based containers
2. WebAssembly (WASM) containers
3. Hybrid containers consisting of both a native and WASM version of a Function. The container type used depends on the Function's invocation policy.

Note that fecore differentiates Function types through a naming suffix. Native Function names should end with `-n`, WASM Function names should end with `-w`, and Hybrid Function names should end with `-h`.

## Deploying Functions

#### Native Functions

Native Functions rely on traditional process-based containers to sandbox Function replicas. A Function's image is retrieved from a container registry (e.g., the Docker Hub public registry or a self-hosted private Docker registry).

To deploy a Native Function:
```
faas-cli -g 10.62.0.1:8081 deploy --image url.to.container.registry/example:latest --name example-n --label ctrType=native
```

#### WebAssembly Functions

WebAssembly (WASM) Functions rely on a WebAssembly runtime to sandbox Function replicas. Function code is executed via the WasmEdge WebAssembly runtime engine. WASM Functions must be packed into a `.tgz` file and copied to `/mnt/faasedge/images`. This `.tgz.` file is unpacked when the image is deployed.

Deploy a WebAssembly Function:
```
faas-cli -g 10.62.0.1:8081 deploy --image image-file-name --name example-w --label ctrType=wasm
```
Note: `image-file-name` refers to the name of the `.tgz` file 

#### Hybrid Functions

Hybrid Functions consist of a Native version and a WebAssembly version, each of which are invoked dynamically in accordance with a policy. For example, a Hybrid Function may have cold starts served with the WebAssembly version (to achieve better startup speed) and warm starts served with the Native version (to achieve better execution speed).

To deploy a Hybrid Function:
- First ensure you deploy the Native and WASM version of the Function as described earlier in this section.
- Then create the Hybrid function with `faas-cli -g 10.62.0.1:8081 deploy --image hybrid --name example-h --label ctrType=hybrid --label sandboxes=example-n,example-w`

## Invoking Functions

Functions can be invoked via an endpoint created by fecore, e.g.:
```
curl -vk http://10.62.0.1:8081/function/example-n
```
