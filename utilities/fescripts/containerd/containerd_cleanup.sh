#!/bin/bash

########################################################
# Manually remove all containerd tasks and containers
# in the specified namespace
########################################################

NAMESPACE="faasedge-fn"

echo "[+] Removing all containerd tasks in namespace '${NAMESPACE}'"
sudo ctr -n ${NAMESPACE} t ls -q | xargs -L 1 ./remove_task.sh

echo "[+] Removing all containerd containers in namespace '${NAMESPACE}'"
sudo ctr -n ${NAMESPACE} c ls -q | xargs -L 1 ./remove_container.sh
