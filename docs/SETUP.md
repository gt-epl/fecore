# Setup fecore

This file contains documentation for setting up dependencies required to compile/run fecore. Be sure to review this entire document before attempting setup to avoid wasted time.

## OS Installation

fecore has been tested on Ubuntu Server 20.04.3 LTS. We recommend using this distribution to avoid dependency issues.

Most importantly, **if you plan to use the WASM/Hybrid functionality of fecore, note the following**: WASM support requires a filesystem that supports reflinks (e.g., btrfs, XFS). Ubuntu defaults to using ext4, so to enable WASM support you will need to use one of the following options:
1. Use btrfs or XFS for the root filesystem (`/`)
2. Add a separate btrfs or XFS partition that will hold WASM files.

If you only plan on using native process-based containers, a standard Ubuntu install on the ext4 filesystem should be fine.

## Installing Dependencies

#### Install Go v1.21.0
```
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

#### Install packages 
`sudo apt-get update && sudo apt-get install git build-essential curl apt-transport-https docker.io runc bridge-utils iptables btrfs-progs`

#### Install CNI v1.3.0
```
wget https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-amd64-v1.3.0.tgz
sudo mkdir -p /opt/cni/bin
sudo tar -C /opt/cni/bin -xzvf cni-plugins-linux-amd64-v1.3.0.tgz
```

#### Install containerd v1.6.16
- `wget https://github.com/containerd/containerd/releases/download/v1.6.16/containerd-1.6.16-linux-amd64.tar.gz`
- `sudo tar -C /usr/local -xzvf containerd-1.6.16-linux-amd64.tar.gz`
- Create a systemd service file for containerd at `/usr/lib/systemd/system/containerd.service` with the following contents:

```
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
```

- Reload systemd: `sudo systemctl daemon-reload`
- Enable the containerd service: `sudo systemctl enable containerd.service`
- Start the containerd service: `sudo systemctl start containerd.service`

#### Setup Network/Firewall

Run the following commands to set up fecore bridge interfaces and firewall rules:
```
# Setup bridge devices
sudo ip link add fecore0 type bridge
sudo ip link set fecore0 up
sudo ip link add fecore0 type bridge
sudo ip addr add 10.62.0.1/16 dev fecore0

sudo ip link add fewasm0 type bridge
sudo ip link set fewasm0 up
sudo ip link add fewasm0 type bridge
sudo ip addr add 10.63.0.1/16 dev fewasm0

# Setup ipmasq rules
# This replaces what the 'host-local' plugin would normally do
sudo iptables -t nat -N fecore-CNI

# Native Networking
sudo iptables -t nat -A POSTROUTING -s 10.62.0.0/16 -m comment --comment "name: \"fecore-cni-bridge\" id: \"fecore-containers\"" -j fecore-CNI
sudo iptables -t nat -A fecore-CNI -d 10.62.0.0/16 -m comment --comment "name: \"fecore-cni-bridge\" id: \"fecore-containers\"" -j ACCEPT
sudo iptables -t nat -A fecore-CNI ! -d 224.0.0.0/4 -m comment --comment "name: \"fecore-cni-bridge\" id: \"fecore-containers\"" -j MASQUERADE

# WASM Networking
sudo iptables -t nat -A POSTROUTING -s 10.63.0.0/16 -m comment --comment "name: \"fecore-wasm-cni-bridge\" id: \"fecore-wasm-containers\"" -j fecore-CNI
sudo iptables -t nat -A fecore-CNI -d 10.63.0.0/16 -m comment --comment "name: \"fecore-wasm-cni-bridge\" id: \"fecore-wasm-containers\"" -j ACCEPT

# Setup firewall rules
# This replaces what the 'firewall' CNI plugin would normally do
sudo iptables -N CNI-ADMIN
sudo iptables -N CNI-FORWARD

sudo iptables -A FORWARD -m comment --comment "CNI firewall plugin rules" -j CNI-FORWARD
sudo iptables -A CNI-FORWARD -m comment --comment "CNI firewall plugin admin overrides" -j CNI-ADMIN

# Native subnets
sudo iptables -A CNI-FORWARD -d 10.62.0.0/16 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
sudo iptables -A CNI-FORWARD -s 10.62.0.0/16 -j ACCEPT

# WASM subnets
sudo iptables -A CNI-FORWARD -d 10.63.0.0/16 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
sudo iptables -A CNI-FORWARD -s 10.63.0.0/16 -j ACCEPT
```

#### Create /mnt/faasedge and fecore configs

fecore uses `/mnt/faasedge` for its config file, log file output, and WASM image location.

NOTE: If you are using WASM functionality and have created a separate btrfs/XFS partition, you will need to mount that partition at `/mnt/faasedge`. It should contain the same directory structure as shown below. You should also add an entry in `/etc/fstab` so that the `/mnt/faasedge` partition gets mounted at boot time.
If you have formatted the entire root filesystem (`/`) with btrfs/XFS, then you can just create `/mnt/faasedge` as a regular directory.

Create the following directories:
```
sudo mkdir /mnt/faasedge
sudo mkdir /mnt/faasedge/logs
sudo mkdir /mnt/faasedge/images
```

Create a fecore config file at `/mnt/faasedge/feconfig.json`. You can find the current schema in `pkg/provider/config/fecore_config.go`. If you do not create this file, fecore will use default config options. The following is an example of how `feconfig.json` should look:
```
{
  "MaxWasmContainers": 500,
  "MaxNativeContainers": 500,
  "RPSEpoch": 10,
  "InvocationSampleThreshold": 100,
  "ContainerCleanupInterval": 10,
  "ContainerExpirationTime": 60,
  "DefaultLogLevel": 2,
  "CurrLogLevel": 3
}
```

Create `/var/lib/fecore/hosts` with the following contents:
```
127.0.0.1       localhost
10.62.0.1       provider
```

Create `/var/lib/fecore/resolv.conf` with the following contents:
```
nameserver 8.8.8.8
```

Create the `secrets` directory:
```
sudo mkdir /var/lib/fecore/secrets
```

#### Setup Private Container Registry (optional)
If you plan on using a private container registry (i.e., one that requires auth to pull images), add a Docker-style config file at `/var/lib/fecore/.docker/config.json`.
The file's format will look like the following:
```
{
  "auths": {
    "hostname.of.your.private.registry": {
    "auth": "AuthKeyForYourRegistry=="
    }
  }
}
```

If you have used Docker to login to a registry before, you should be able to find a file with similar contents at `$HOME/.docker/config.json`.

## WASM Setup

First, ensure you have a disk partition formatted with a filesystem that supports reflinks, such as btrfs or XFS. This filesystem should be mounted at `/mnt/faasedge`. The filesystem is used by fecore to enable Copy-on-Write for files within a WebAssembly Function's scratch disk space.

Next, you will need to install the WasmEdge v0.12.1 runtime:
```
# Install the runtime itself (required)
curl -sSf https://raw.githubusercontent.com/WasmEdge/WasmEdge/master/utils/install.sh | bash -s -- -v 0.12.1
```

The preceding command should place installation files in your `$HOME/.wasmedge` folder. 
You'll need to move the .so files in `lib` to `/usr/local/lib` and the binaries in `bin` to `/usr/local/bin`:
```
sudo cp -av $HOME/.wasmedge/bin/* /usr/local/bin
sudo cp -av $HOME/.wasmedge/lib/*.so* /usr/local/lib
sudo ldconfig
```

#### Setup cgroups

By default, each WASM Function replica is cgroups-restricted to 1 share of CPU and can only run on a specified range of CPUs.
Set up cgroups for WASM functions with the following commands:
```
sudo mkdir /sys/fs/cgroup/cpu/fewasm
sudo mkdir /sys/fs/cgroup/cpuset/fewasm
sudo mkdir /sys/fs/cgroup/memory/fewasm
echo "0" | sudo tee /sys/fs/cgroup/cpuset/fewasm/cpuset.mems
# Restrict WASM functions to using cpu0-15
# Note you likely need to update this depending on the CPU layout of the installed system
echo "0-15" | sudo tee /sys/fs/cgroup/cpuset/fewasm/cpuset.cpus
```

#### Create WASM Network Interfaces

The `wasmnet` utility can be used to pre-create network interfaces that will be used by WASM function replicas.
The following command will create 500 interfaces by default:

`sudo utilities/wasmnet/wasmnet`

Note: This will need to be done each time the system is rebooted.

#### Add Example WASM Images
fecore expects to find images for wasm files in .tgz format at `/mnt/faasedge/images` (e.g., `/mnt/faasedge/images/wasm_function.tgz`).
When a WASM function is initially deployed, fecore will unpack its associated .tgz file.
You will need to add any images you wish to deploy to this directory.