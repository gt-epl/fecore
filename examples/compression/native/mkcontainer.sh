#!/bin/bash

echo "[+] Copying input files to current dir"
cp ../inputs/input.file .

echo "[+] Building container"
sudo buildah build --cgroupns private --ipc private --network private --pid private --userns container --uts private --isolation oci --cap-drop cap_net_admin --security-opt seccomp=/usr/share/containers/seccomp.json -t compression-n .

rm input.file

echo "[+] Done."
