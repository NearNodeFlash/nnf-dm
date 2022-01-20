#!/bin/bash

# Create the Data Movement request
CMD=${1:-lustre}

echo "$(tput bold)Creating override config $(tput sgr 0)"
kubectl delete configmap/data-movement-mpi-config &> /dev/null
cat <<-EOF | kubectl create -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: data-movement-mpi-config
  namespace: default
data:
  image: arti.dev.cray.com/rabsw-docker-master-local/mfu:0.0.1
  sourceVolume:      '{ "hostPath": { "path": "/nnf", "type": "Directory" } }'
  destinationVolume: '{ "hostPath": { "path": "/nnf", "type": "Directory" } }'
EOF

if [[ $CMD == "lustre" ]]; then
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

