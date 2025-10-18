#!/bin/bash

NAMESPACE="faasedge-fn"
CONTAINER_NAME=$1

###############################################
# Removes a container created by containerd   # 
###############################################

sudo ctr -n ${NAMESPACE} c rm ${CONTAINER_NAME}

