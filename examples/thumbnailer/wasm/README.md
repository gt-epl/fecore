# thumbnailer wasm

```
LATEST BUILD
2024-02-13
thumbnailer-w.wasm md5=8add1e9a06cf3979ce0df4462e2dca3a
```

Image thumbnail creation in WASM. Takes as input a PNG image and outputs a thumbnail of that image in PNG format.

Note: The input for this image is hardcoded into the WASM binary.

This is a standalone (i.e., no built-in HTTP server) binary. It requires `runw` or similar to be invoked via network.

Adapted from the [Trivernis/thumbnailer](https://github.com/Trivernis/thumbnailer.git) repo.

Note: This repository is a Rust library for creating image thumbnails. We build an example using this library.

## Build

The source for our example program is located in `examples/thumbnailer.rs`.

Build `thumbnailer.wasm` with:
```
cargo build --release --example thumbnailer --target wasm32-wasi
```

Be sure to optimize the WASM file after building it:
```
wasmedgec --optimize 3 ./target/wasm32-wasi/release/examples/thumbnailer.wasm thumbnailer-w.wasm
```

## Run

Run with wasmedge:
```
wasmedge --dir .:. ./thumbnailer-w.wasm
```

## Container Creation
1. Create a directory for the image with the following layout:
```
thumbnailer-w
|- function.wasm
|- replicas
|- rootfs
```
2. Bundle the image directory: `tar cvvzf thumbnailer-w.tgz thumbnailer-w`
3. Move the image bundle .tgz to /mnt/faasedge/images (e.g., `mv thumbnailer-w.tgz /mnt/faasedge/images/thumbnailer-w.tgz`)

## Running with fecore

1. Start fecore
2. Deploy the function: 
```
faas-cli -g 10.62.0.1:8081 deploy --image=thumbnailer-w --name thumbnailer-w --label ctrType=wasm
```
3. Invoke the function:
```
curl http://10.62.0.1:8081/function/thumbnailer-w
```
