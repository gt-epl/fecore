#!/bin/bash

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
echo "[+] Done"
echo ""
echo "[!] Remember to create WASM network namespaces and veth interfaces by running 'wasmnet' if you plan on using WASM containers via fecore"

