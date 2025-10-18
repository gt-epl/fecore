#!/bin/bash

SETUP_DIR=$(pwd)
INSTALL_DISK="/dev/sdc"

# Install dependencies
sudo apt-get update
sudo apt-get install git build-essential curl apt-transport-https docker.io runc bridge-utils iptables btrfs-progs -y

# Add current user to docker group
sudo usermod -aG docker $USER

# Set up a btrfs partition to hold RUNE files
sudo mkdir -p /mnt/faasedge/{images,logs,bin}
echo 'start=2048, type=83' | sudo sfdisk ${INSTALL_DISK}
sudo mkfs.btrfs ${INSTALL_DISK}1
sudo mount ${INSTALL_DISK}1 /mnt/faasedge
sudo chown -R $USER /mnt/faasedge

# Copy binaries and config to mnt/faasedge
sudo cp bin/fecore /usr/local/bin/
cp bin/wasmnet /mnt/faasedge/bin/
sudo cp bin/faas-cli /usr/local/bin/
cp etc/feconfig.json /mnt/faasedge/

# Install buildah
cd ${SETUP_DIR}/bin/buildah/
sudo cp buildah /usr/bin/buildah
sudo cp libgpgme.so.11 /lib/x86_64-linux-gnu/libgpgme.so.11
sudo mkdir /etc/containers 
sudo tar -xzf etc-containers.tgz -C /etc/containers/
sudo mkdir /usr/share/containers 
sudo tar -xzf usr-share-containers.tgz -C /usr/share/containers/

# Install Go
cd ${SETUP_DIR}/bin/
# wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
# rm -f go1.21.0.linux-amd64.tar.gz

# Install CNI
cd ${SETUP_DIR}/bin/
# wget https://github.com/containernetworking/plugins/releases/download/v0.9.1/cni-plugins-linux-amd64-v0.9.1.tgz
sudo mkdir -p /opt/cni/bin
sudo tar -C /opt/cni/bin -xzvf cni-plugins-linux-amd64-v1.3.0.tgz
# rm -f cni-plugins-linux-amd64-v1.3.0.tgz

# Install containerd
cd ${SETUP_DIR}/bin/
# wget https://github.com/containerd/containerd/releases/download/v1.6.16/containerd-1.6.16-linux-amd64.tar.gz
sudo tar -C /usr/local -xzvf containerd-1.6.16-linux-amd64.tar.gz
# rm -f containerd-1.6.16-linux-amd64.tar.gz

# Setup containerd service
cat << 'EOF' | sudo tee /usr/lib/systemd/system/containerd.service > /dev/null
[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target local-fs.target

[Service]
#uncomment to enable the experimental sbservice (sandboxed) version of containerd/cri integration
#Environment="ENABLE_CRI_SANDBOXES=sandboxed"
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/containerd
# Remove the preceding line and uncomment the below line to restrict containerd to cpus1-4
#ExecStart=taskset -a -c 1-4 /usr/local/bin/containerd

Type=notify
Delegate=yes
KillMode=process
Restart=always
RestartSec=5
# Having non-zero Limit*s causes performance problems due to accounting overhead
# in the kernel. We recommend using cgroups to do container-local accounting.
LimitNPROC=infinity
LimitCORE=infinity
LimitNOFILE=infinity
# Comment TasksMax if your systemd version does not supports it.
# Only systemd 226 and above support this version.
TasksMax=infinity
OOMScoreAdjust=-999

[Install]
WantedBy=multi-user.target
[Manager]
DefaultTimeoutStartSec=3m
EOF

sudo systemctl daemon-reload
sudo systemctl enable containerd.service
sudo systemctl start containerd.service

# Set up networking
FECORE_BRIDGE="fecore0"
FECORE_IP="10.62.0.1/16"

FEWASM_BRIDGE="fewasm0"
FEWASM_IP="10.63.0.1/16"

FECORE_SUBNET="10.62.0.0/16"
FEWASM_SUBNET="10.63.0.0/16"

# Setup bridge devices
echo "[+] Adding bridge device '${FECORE_BRIDGE}' with subnet ${FECORE_IP} for fecore native containers"
sudo ip link add ${FECORE_BRIDGE} type bridge
sudo ip link set ${FECORE_BRIDGE} up
sudo ip link add ${FECORE_BRIDGE} type bridge
sudo ip addr add ${FECORE_IP} dev ${FECORE_BRIDGE}

echo "[+] Adding bridge device '${FEWASM_BRIDGE}' with subnet ${FEWASM_IP} for fecore WASM containers"
sudo ip link add ${FEWASM_BRIDGE} type bridge
sudo ip link set ${FEWASM_BRIDGE} up
sudo ip link add ${FEWASM_BRIDGE} type bridge
sudo ip addr add ${FEWASM_IP} dev ${FEWASM_BRIDGE}

echo "[+] Adding firewall rules for fecore and fewasm networks"
# Setup ipmasq rules
# This replaces what the 'host-local' plugin would normally do
sudo iptables -t nat -N fecore-CNI

# Native Networking
sudo iptables -t nat -A POSTROUTING -s ${FECORE_SUBNET} -m comment --comment "name: \"fecore-cni-bridge\" id: \"fecore-containers\"" -j fecore-CNI
sudo iptables -t nat -A fecore-CNI -d ${FECORE_SUBNET} -m comment --comment "name: \"fecore-cni-bridge\" id: \"fecore-containers\"" -j ACCEPT

# WASM Networking
sudo iptables -t nat -A POSTROUTING -s ${FEWASM_SUBNET} -m comment --comment "name: \"fecore-wasm-cni-bridge\" id: \"fecore-wasm-containers\"" -j fecore-CNI
sudo iptables -t nat -A fecore-CNI -d ${FEWASM_SUBNET} -m comment --comment "name: \"fecore-wasm-cni-bridge\" id: \"fecore-wasm-containers\"" -j ACCEPT

# Deny multicast
sudo iptables -t nat -A fecore-CNI ! -d 224.0.0.0/4 -m comment --comment "name: \"fecore-cni-bridge\" id: \"fecore-containers\"" -j MASQUERADE

# Setup firewall rules
# This replaces what the 'firewall' CNI plugin would normally do
sudo iptables -N CNI-ADMIN
sudo iptables -N CNI-FORWARD

sudo iptables -A FORWARD -m comment --comment "CNI firewall plugin rules" -j CNI-FORWARD
sudo iptables -A CNI-FORWARD -m comment --comment "CNI firewall plugin admin overrides" -j CNI-ADMIN

# Native subnets
sudo iptables -A CNI-FORWARD -d ${FECORE_SUBNET} -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
sudo iptables -A CNI-FORWARD -s ${FECORE_SUBNET} -j ACCEPT

# WASM subnets
sudo iptables -A CNI-FORWARD -d ${FEWASM_SUBNET} -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
sudo iptables -A CNI-FORWARD -s ${FEWASM_SUBNET} -j ACCEPT

sudo /mnt/faasedge/bin/wasmnet

# Create dir for fecore files
sudo mkdir /var/lib/fecore

# Create fecore hosts
cat << 'EOF' | sudo tee /var/lib/fecore/hosts > /dev/null
127.0.0.1       localhost
10.62.0.1       provider
EOF

# Create fecore resolv.conf
cat << 'EOF' | sudo tee /var/lib/fecore/resolv.conf > /dev/null
nameserver 8.8.8.8
EOF

# Create fecore secrets dir
sudo mkdir /var/lib/fecore/secrets

# Set up WasmEdge runtime
cd ${SETUP_DIR}
#curl -sSf https://raw.githubusercontent.com/WasmEdge/WasmEdge/master/utils/install.sh | bash -s -- -v 0.12.1
sudo cp -av bin/WasmEdge/bin/* /usr/local/bin
sudo cp -av bin/WasmEdge/lib/*.so* /usr/local/lib
sudo ldconfig

# Set up cgroups for WASM
sudo mkdir /sys/fs/cgroup/cpu/fewasm
sudo mkdir /sys/fs/cgroup/cpuset/fewasm
sudo mkdir /sys/fs/cgroup/memory/fewasm
echo "0" | sudo tee /sys/fs/cgroup/cpuset/fewasm/cpuset.mems
# Restrict WASM functions to using cpu0-15
# Note you likely need to update this depending on the CPU layout of the installed system
echo "0-15" | sudo tee /sys/fs/cgroup/cpuset/fewasm/cpuset.cpus

# Set up example containers
cd ${SETUP_DIR}

# Compression setup
echo "--- Compression Example Setup ---"
cd examples/compression
# compression-n
echo "  [!] Setting up Compression native container"
cd native
echo "    [+] Building container"
sudo buildah build --cgroupns private --ipc private --network private --pid private --userns container --uts private --isolation oci --cap-drop cap_net_admin --security-opt seccomp=/usr/share/containers/seccomp.json -t compression-n .
echo "    [+] Done."
cd ..

# compression-w
echo "  [!] Setting up Compression wasm container"
cd wasm
echo "    [+] Moving image archive to /mnt/faasedge/images"
cp compression-w.tgz /mnt/faasedge/images
echo "    [+] Done."
cd ${SETUP_DIR}

echo " "

# Encrypt setup
echo "--- Encrypt Example Setup ---"
cd examples/encrypt
# encrypt-n
echo "  [!] Setting up Encrypt native container"
cd native
echo "    [+] Building container"
sudo buildah build --cgroupns private --ipc private --network private --pid private --userns container --uts private --isolation oci --cap-drop cap_net_admin --security-opt seccomp=/usr/share/containers/seccomp.json -t encrypt-p .
echo "    [+] Done."
cd ..

# encrypt-w
echo "  [!] Setting up Encrypt wasm container"
cd wasm
echo "    [+] Moving image archive to /mnt/faasedge/images"
cp encrypt-w.tgz /mnt/faasedge/images
echo "[+] Done."
cd ${SETUP_DIR}

# Thumbnailer setup
echo "--- Thumbnailer Example Setup ---"
cd examples/thumbnailer
# thumbnailer-n
echo "  [!] Setting up Thumbnailer native container"
cd native
echo "    [+] Building container"
sudo buildah build --cgroupns private --ipc private --network private --pid private --userns container --uts private --isolation oci --cap-drop cap_net_admin --security-opt seccomp=/usr/share/containers/seccomp.json -t thumbnailer-j .
echo "    [+] Done."
cd ..

# thumbnailer-w
echo "  [!] Setting up Thumbnailer wasm container"
cd wasm
echo "    [+] Moving image archive to /mnt/faasedge/images"
cp thumbnailer-w.tgz /mnt/faasedge/images
echo "[+] Done."

cd /mnt/faasedge
