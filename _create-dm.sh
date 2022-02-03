#!/bin/bash


if [ $# -eq 0 ]; then
  echo "Create DM requires one of 'lustre' or 'xfs'"
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
    namespace: default
  data:
    image: arti.dev.cray.com/rabsw-docker-master-local/mfu:0.0.1
    sourceVolume:      '{ "hostPath": { "path": "/nnf", "type": "Directory" } }'
    destinationVolume: '{ "hostPath": { "path": "/nnf", "type": "Directory" } }'
EOF


  echo "$(tput bold)Creating lustre data movement $(tput sgr 0)"
  cat <<-EOF | kubectl apply -f -
  apiVersion: dm.cray.hpe.com/v1alpha1
  kind: DataMovement
  metadata:
    name: datamovement-sample-lustre-3
    namespace: nnf-dm-system
  spec:
    source:
      path: /file.in
      storageInstance:
        kind: LustreFileSystem
        name: lustrefilesystem-sample-maui
        namespace: default
    destination:
      path: /file.out
      storageInstance:
        kind: NnfJobStorageInstance
        name: nnfjobstorageinstance-sample-lustre
        namespace: default
    storage:
      kind: NnfStorage
      name: nnfstorage-sample
      namespace: default
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
    namespace: default
  data:
    sourcePath: "/mnt/nnf/file.in"
    destinationPath: "/mnt/nnf/file.out"
EOF


  echo "$(tput bold)Creating rsync data movement $(tput sgr 0)"
  cat <<-EOF | kubectl apply -f -
  apiVersion: dm.cray.hpe.com/v1alpha1
  kind: DataMovement
  metadata:
    name: datamovement-sample-xfs
    namespace: nnf-dm-system
  spec:
    source:
      path: file.in
      storageInstance:
        kind: LustreFileSystem
        name: lustrefilesystem-sample-maui
        namespace: default
    destination:
      path: file.out
      storageInstance:
        kind: NnfJobStorageInstance
        name: nnfjobstorageinstance-sample
        namespace: default
    storage:
      kind: NnfStorage
      name: nnfstorage-sample
      namespace: default
EOF

fi
