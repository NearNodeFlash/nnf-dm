#!/bin/bash

if [ $# -eq 0 ]; then
  echo "Override Config requires one of 'lustre' or 'xfs'"
  exit 1
fi

CMD=$1


if [[ "$CMD" == lustre ]]; then
  echo "$(tput bold)Creating override config $(tput sgr 0)"
  kubectl delete configmap/data-movement-config &> /dev/null
  cat <<-EOF | kubectl create -f -
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: data-movement-config
    namespace: nnf-dm-system
  data:
    image: arti.dev.cray.com/rabsw-docker-master-local/mfu:0.0.1
    sourceVolume:      '{ "hostPath": { "path": "/nnf", "type": "Directory" } }'
    destinationVolume: '{ "hostPath": { "path": "/nnf", "type": "Directory" } }'
EOF
fi

if [[ "$CMD" == xfs ]]; then
  echo "$(tput bold)Creating override config $(tput sgr 0)"
  kubectl delete configmap/data-movement-config &> /dev/null
  cat <<-EOF | kubectl create -f -
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: data-movement-config
    namespace: nnf-dm-system
  data:
    sourcePath: "/mnt/nnf/file.in"
    destinationPath: "/mnt/nnf/file.out"
EOF
fi