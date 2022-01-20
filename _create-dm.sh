#!/bin/bash

shopt -s expand_aliases

# Create the Data Movement request
CMD=$1

echo "$(tput bold)Creating override config$(tput sgr 0)"
kubectl delete configmap/data-movement-mpi-config &> /dev/null
cat <<-EOF | kubectl create -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: data-movement-mpi-config
  namespace: default
data:
  image: arti.dev.cray.com/rabsw-docker-master-local/mfu:0.0.1
  command: touch /mnt/src/file.in && cp
  sourceVolume:      '{ "hostPath": { "path": "/nnf/src",  "type": "Directory" } }'
  destinationVolume: '{ "hostPath": { "path": "/nnf/dest", "type": "Directory" } }'
EOF

if [[ $CMD == "lustre" ]]; then
    echo "$(tput bold)Creating lustre data movement$(tput sgr 0)"
    kubectl apply -f config/samples/lustre/dm_v1alpha1_datamovement.yaml
fi

