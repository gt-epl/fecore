#!/bin/bash

NAMESPACE="faasedge-fn"
TASK_NAME=$1

###############################################
# Removes a task created by containerd
###############################################

sudo ctr -n ${NAMESPACE} t kill -s 9 ${TASK_NAME}
