#!/bin/bash

# Setup the current cluster with the resources necessary to run a data-movement resource

# Install the prerequisite CRDs
echo "$(tput bold)Installing prerequisite CRDs $(tput sgr 0)"
kubectl apply -f vendor/github.hpe.com/hpe/hpc-rabsw-nnf-sos/config/crd/bases/nnf.cray.hpe.com_nnfjobstorageinstances.yaml
kubectl apply -f vendor/github.hpe.com/hpe/hpc-rabsw-nnf-sos/config/crd/bases/nnf.cray.hpe.com_nnfstorages.yaml
kubectl apply -f vendor/github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/config/crd/bases/cray.hpe.com_lustrefilesystems.yaml
kubectl apply -f config/mpi/mpi-operator.yaml

# Install the sample resources
echo "$(tput bold)Installing sample resources $(tput sgr 0)"
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

cat <<-EOF | kubectl apply -f -
apiVersion: nnf.cray.hpe.com/v1alpha1
kind: NnfJobStorageInstance
metadata:
  name: nnfjobstorageinstance-sample-lustre
spec:
  name: my-job-storage
  fsType: lustre
  servers:
    kind: NnfStorage
    name: nnfstorage-sample-lustre
    namespace: default
EOF

# For NNFStorage we need to program the mgsNode value which is under the status section; making it unprogrammable by default.
# Edit the CRD definition to remove the status section as a subresource.
kubectl get crd/nnfstorages.nnf.cray.hpe.com -o json | jq 'del(.spec.versions[0].subresources)' | kubectl apply -f -

echo "$(tput bold)Deploying data movement$(tput sgr 0)"
make deploy

echo "$(tput bold)DONE!$(tput sgr 0)"

