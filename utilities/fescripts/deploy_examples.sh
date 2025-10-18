#!/bin/bash

##############################################################
# Deploys fecore example Native, WASM, and Hybrid Functions  #
##############################################################

echo "[+] Deploying thumbnailer native/wasm/hybrid"
faas-cli -g 10.62.0.1:8081 deploy --image achgt/thumbnailer-n:latest --name thumbnailer-n --label ctrType=native
faas-cli -g 10.62.0.1:8081 deploy --image=thumbnailer-w --name thumbnailer-w --label ctrType=wasm
faas-cli -g 10.62.0.1:8081 deploy --image hybrid --name thumbnailer-h --label ctrType=hybrid --label sandboxes=thumbnailer-n,thumbnailer-w

echo "[+] Deploying encrypt native/wasm/hybrid"
faas-cli -g 10.62.0.1:8081 deploy --image achgt/encrypt-p:latest --name encrypt-n --label ctrType=native
faas-cli -g 10.62.0.1:8081 deploy --image=encrypt-w --name encrypt-w --label ctrType=wasm
faas-cli -g 10.62.0.1:8081 deploy --image hybrid --name encrypt-h --label ctrType=hybrid --label sandboxes=encrypt-n,encrypt-w

echo "[+] Deploying compression native/wasm/hybrid"
faas-cli -g 10.62.0.1:8081 deploy --image achgt/compression-n:latest --name compression-n --label ctrType=native
faas-cli -g 10.62.0.1:8081 deploy --image=compression-w --name compression-w --label ctrType=wasm
faas-cli -g 10.62.0.1:8081 deploy --image hybrid --name compression-h --label ctrType=hybrid --label sandboxes=compression-n,compression-w

