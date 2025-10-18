#!/bin/bash

echo "[+] Creating tmp dir for thumbnailer-w"
mkdir -p tmp/thumbnailer-w
echo "[+] Creating replicas dir"
mkdir tmp/thumbnailer-w/replicas
echo "[+] Creating rootfs dir"
mkdir tmp/thumbnailer-w/rootfs

echo "[+] Copying thumbnailer-w.wasm to function.wasm"
cp thumbnailer-w.wasm tmp/thumbnailer-w/function.wasm

echo "[+] Creating thumbnailer-w.tgz archive"
cd tmp
tar cvvzf thumbnailer-w.tgz thumbnailer-w

echo "[+] Moving image archive to /mnt/faasedge/images"
mv thumbnailer-w.tgz /mnt/faasedge/images

echo "[+] Cleaning up tmp"
cd ..
rm -rf ./tmp

echo "[+] Done."
