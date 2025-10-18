#!/bin/bash

WASM_CPUS="0-15"

echo "[+] Creating 'cpu' cgroup for fecore wasm containers"
sudo mkdir /sys/fs/cgroup/cpu/fewasm
echo "[+] Creating 'cpuset' cgroup for fecore wasm containers"
sudo mkdir /sys/fs/cgroup/cpuset/fewasm
echo "[+] Setting cpuset to CPUs ${WASM_CPUS}. Please ensure this range of CPUs is valid for your system."
echo "0" | sudo tee /sys/fs/cgroup/cpuset/fewasm/cpuset.mems
echo ${WASM_CPUS} | sudo tee /sys/fs/cgroup/cpuset/fewasm/cpuset.cpus

