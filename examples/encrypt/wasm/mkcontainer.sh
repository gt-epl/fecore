#!/bin/bash

echo "[+] Creating tmp dir for encrypt-w"
mkdir -p tmp/encrypt-w
echo "[+] Creating replicas dir"
mkdir tmp/encrypt-w/replicas
echo "[+] Creating rootfs dir"
mkdir tmp/encrypt-w/rootfs

echo "[+] Copying encrypt-w.wasm to function.wasm"
cp encrypt-w.wasm tmp/encrypt-w/function.wasm

echo "[+] Creating encrypt-w.tgz archive"
cd tmp
tar cvvzf encrypt-w.tgz encrypt-w

echo "[+] Moving image archive to /mnt/faasedge/images"
mv encrypt-w.tgz /mnt/faasedge/images

echo "[+] Cleaning up tmp"
cd ..
rm -rf ./tmp

echo "[+] Done."
