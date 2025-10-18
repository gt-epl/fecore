#!/bin/bash

echo "[+] Creating tmp dir for compression-w"
mkdir -p tmp/compression-w
echo "[+] Creating replicas dir"
mkdir tmp/compression-w/replicas
echo "[+] Creating rootfs dir"
mkdir tmp/compression-w/rootfs

echo "[+] Copying compression-w.wasm to function.wasm"
cp compression-w.wasm tmp/compression-w/function.wasm

echo "[+] Copying libz.a to rootfs"
cp libz.a tmp/compression-w/rootfs

echo "[+] Copying input files to rootfs"
cp ../inputs/* tmp/compression-w/rootfs

echo "[+] Creating compression-w.tgz archive"
cd tmp
tar cvvzf compression-w.tgz compression-w

echo "[+] Moving image archive to /mnt/faasedge/images"
mv compression-w.tgz /mnt/faasedge/images

echo "[+] Cleaning up tmp"
cd ..
rm -rf ./tmp

echo "[+] Done."
