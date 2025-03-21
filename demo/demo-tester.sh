#!/bin/bash

# For htx-1

unset https_proxy

SECRET_NAME=$(kubectl get workflow copy-offload-blake -ojson | jq -rM .status.workflowToken.secretName)
kubectl get secret "$SECRET_NAME" -ojson | jq -rM .data.token | base64 -d > /root/token
SRC=$(kubectl get workflow copy-offload-blake -ojson | jq -rM '.status.env["DW_JOB_copyoff"]')

export DW_WORKFLOW_NAME=copy-offload-blake
export DW_WORKFLOW_NAMESPACE=default
export DW_WORKFLOW_TOKEN
export NNF_CONTAINER_LAUNCHER
export NNF_CONTAINER_PORTS

DW_WORKFLOW_TOKEN=$(</root/token)
NNF_CONTAINER_LAUNCHER=$(kubectl get workflow copy-offload-blake -ojson | jq -rM '.status.env["NNF_CONTAINER_LAUNCHER"]')
NNF_CONTAINER_PORTS=$(kubectl get workflow copy-offload-blake -ojson | jq -rM '.status.env["NNF_CONTAINER_PORTS"]')


fallocate -l 2GB "$SRC"/durian-"$(hostname)"

./tester \
	-o \
	-S "$SRC" \
	-D /lus/global/copyoff \
	-P no-xattr \
        -m 32

