# runw

This dir contains source files for the *runw* utility that acts as a proxy and loader for WebAssembly binaries.

`runw_web.c` has a built-in webserver that listens on port 8080 and invokes the WASM code when a client connects.

It drops into a pre-created network namespace before executing the WASM code.

By default, the code will *not* load WasmEdge plugins (e.g., WasiNN).

## Building

This version of runw requires WasmEdge v0.12.1

Build with the following command:
`gcc runw_web.c -lwasmedge -o runw`

To enable WasmEdge plugins, build with the following command instead:
`gcc runw_web.c -DWITH_PLUGINS -lwasmedge -o runw`
