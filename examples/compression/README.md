# zpipe

Implementation of [example code](https://www.zlib.net/zpipe.c) from zlib.

Compresses stdin via DEFLATE compression algorithm (used in gzip).

Currently zpipe is hardcoded to compress a file named "input.file".

## Native

Build using the included Makefile. No external deps should be required (zlib source and libz.a are already included in this repo).

Usage:

`./zpipe`


## WASM

To build, we use the [wasi-sdk](https://github.com/WebAssembly/wasi-sdk) Docker container from `ghcr.io/webassembly/wasi-sdk`.

Enter the container with the following command from the `zpipe` dir:

`docker run -it -v `pwd`:/src -w /src ghcr.io/webassembly/wasi-sdk`

From inside the container:
- cd wasm
- make

Remember to perform AOT optimization on the resulting `.wasm` binary if you want to achieve full performance:

`wasmedgec --optimize 3 zpipe.wasm zpipe-aot.wasm`

Usage:

`wasmedge --dir .:. ./zpipe-aot.wasm`


## Input Files

Tested using a 3.2 MB text of [`War & Peace` from Project Gutenberg](https://www.gutenberg.org/cache/epub/2600/pg2600.txt)

## zlib

Builds for native and WASM versions of zlib are included in this repo. If you need to rebuild:

- cd zlib
- make (This will output `libz.a` in the build directory)
- make install (This will place the lib files in `/usr/local/lib`)

(Note: Repeat these same steps inside the `wasi-sdk` container for a WASM build).
