# encrypt wasm

```
LATEST BUILD
2024-02-13
encrypt-w.wasm md5=95e68bf7ccfcaaf6e4a8246518310b8f
```

AES 128-bit CTR encryption in WASM. Encrypts the string `hello world` 10,000 times.

This is a standalone (i.e., no built-in HTTP server) binary. It requires `runw` or similar to be invoked via network.

Adapted from the [rust-aes-wasm](https://github.com/jedisct1/rust-aes-wasm/tree/master) repo.

Note: This repository is a library for performing AES encryption in WASM. We build an example using this library.

## Build

The source for our example program is located in `examples/encrypt.rs`.

Build `encrypt.wasm` with:
```
cargo build --release --example encrypt --target wasm32-wasi
```

Be sure to optimize the WASM file after building it:
```
wasmedgec --optimize 3 ./target/wasm32-wasi/release/examples/encrypt.wasm encrypt-w.wasm
```

## Run

Run with wasmedge:
```
wasmedge ./encrypt-w.wasm
```

## Container Creation
1. Create a directory for the image with the following layout:
```
encrypt-w
|- function.wasm
|- replicas
|- rootfs
```
2. Bundle the image directory: `tar cvvzf encrypt-w.tgz encrypt-w`
3. Move the image bundle .tgz to /mnt/faasedge/images (e.g., `mv encrypt-w.tgz /mnt/faasedge/images/encrypt-w.tgz`)

## Running with fecore

1. Start fecore
2. Deploy the function: 
```
faas-cli -g 10.62.0.1:8081 deploy --image=encrypt-w --name encrypt-w --label ctrType=wasm
```
3. Invoke the function:
```
curl http://10.62.0.1:8081/function/encrypt-w
```
