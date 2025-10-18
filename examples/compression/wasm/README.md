# compression wasm

```
LATEST BUILD
2024-02-13
compression-w.wasm md5=5aa715a888204b40ba9f9f4abeb3186f
```

File compression in WASM. Takes as input a text file and outputs a zlib compressed version.

Note: Hardcoded to expect the input to be named "input.file"

This is a standalone (i.e., no built-in HTTP server) binary. It requires `runw` or similar to be invoked via network.

Adapted from the [zpipe.c example code](https://www.zlib.net/zpipe.c).

Uses [zlib library](https://zlib.net)

## Build

To build, we use the [wasi-sdk](https://github.com/WebAssembly/wasi-sdk) Docker container from `ghcr.io/webassembly/wasi-sdk`.

Enter the container with the following command from the `compression` dir (one level above this one):
```
docker run -it -v `pwd`:/src -w /src ghcr.io/webassembly/wasi-sdk
```

From inside the container:
```
- cd wasm
- make
```

This should produce `zpipe.wasm` and `libz.a`. Exit the container and switch to the `wasm` dir before continuing to the next step.

Be sure to optimize the WASM file after building it:
```
wasmedgec --optimize 3 ./zpipe.wasm compression-w.wasm
```

## Run

Run with wasmedge:
```
wasmedge --dir .:../inputs ./compression-w.wasm
```

## Container Creation
1. Create a directory for the image with the following layout:
```
compression-w
|- function.wasm
|- replicas
|- rootfs
  |- input.file
  |- libz.a
```
2. Bundle the image directory: `tar cvvzf compression-w.tgz compression-w`
3. Move the image bundle .tgz to /mnt/faasedge/images (e.g., `mv compression-w.tgz /mnt/faasedge/images/compression-w.tgz`)

## Running with fecore

1. Start fecore
2. Deploy the function: 
```
faas-cli -g 10.62.0.1:8081 deploy --image=compression-w --name compression-w --label ctrType=wasm
```
3. Invoke the function:
```
curl http://10.62.0.1:8081/function/compression-w
```
