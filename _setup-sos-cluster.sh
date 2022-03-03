#!/bin/bash

# Setup for nnf-sos based kind cluster

echo "$(tput bold)Installing prerequisite CRDs $(tput sgr 0)"
kubectl apply -f config/mpi/mpi-operator.yaml
kubectl apply -f vendor/github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/config/crd/bases/cray.hpe.com_lustrefilesystems.yaml

echo "$(tput bold)Installing sample LustreFileSystem $(tput sgr 0)"
cat <<-EOF | kubectl apply -f -
apiVersion: cray.hpe.com/v1alpha1
kind: LustreFileSystem
metadata:
  name: lustrefilesystem-sample-maui
spec:
  name: maui
  mgsNid: 172.0.0.1@tcp
  mountRoot: /lus/maui
EOF

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

echo "$(tput bold)Running make deploy to install data movement $(tput sgr 0)"
make deploy

if [[ $(kubectl get rsynctemplates -n nnf-dm-system 2>&1) =~ "No resources found" ]]; then
  echo "$(tput bold)Rsync Template not loaded, retrying deployment $(tput sgr 0)"
  make deploy
fi


echo "$(tput bold)Patching rsync template to disable Lustre File Systems $(tput sgr 0)"
echo "This will allow the permit the rsync nodes to start - which is otherwise prevented"
echo "since the Lustre CSI is not loaded"
kubectl get rsynctemplate/nnf-dm-rsynctemplate -n nnf-dm-system -o json | jq '.spec += {"disableLustreFileSystems": true}' | kubectl apply -f -