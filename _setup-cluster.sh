#!/bin/bash

# Copyright 2021, 2022 Hewlett Packard Enterprise Development LP
# Other additional copyright holders may be indicated within.
#
# The entirety of this work is licensed under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
#
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# Setup the current cluster with the resources necessary to run a data-movement resource as a stand alone resource.
# This is useful if you want to run data movement outside of any controlling resource, like nnf-sos manager.

if [ $# -eq 0 ]; then
  echo "Setup Cluster requires one of 'lustre' or 'xfs'"
  exit 1
fi

CMD=$1

# Install the prerequisite CRDs
echo "$(tput bold)Installing prerequisite CRDs $(tput sgr 0)"
kubectl apply -f vendor/github.com/NearNodeFlash/nnf-sos/config/crd/bases/nnf.cray.hpe.com_nnfdatamovements.yaml
kubectl apply -f vendor/github.com/NearNodeFlash/nnf-sos/config/crd/bases/nnf.cray.hpe.com_nnfstorages.yaml
kubectl apply -f vendor/github.com/NearNodeFlash/nnf-sos/config/crd/bases/nnf.cray.hpe.com_nnfaccesses.yaml
kubectl apply -f vendor/github.com/NearNodeFlash/lustre-fs-operator/config/crd/bases/cray.hpe.com_lustrefilesystems.yaml
kubectl apply -f config/mpi/mpi-operator.yaml

# Install the sample resources

# Source is always a global lustre file system
echo "$(tput bold)Installing sample LustreFileSystem $(tput sgr 0)"
cat <<-EOF | kubectl apply -f -
apiVersion: cray.hpe.com/v1alpha1
kind: LustreFileSystem
metadata:
  name: lustrefilesystem-sample-maui
  namespace: nnf-dm-system
spec:
  name: maui
  mgsNids:
  - 172.0.0.1@tcp
  mountRoot: /lus/maui
EOF

echo "$(tput bold)Installing sample NNF Access $(tput sgr 0)"
cat <<-EOF | kubectl apply -f -
  apiVersion: nnf.cray.hpe.com/v1alpha1
  kind: NnfAccess
  metadata:
    name: nnfaccess-sample
  spec:
    desiredState: mounted
    teardownState: data_out
    target: all
    mountPathPrefix: "/mnt"
    storageReference:
      kind: NnfStorage
      name: nnfstorage-sample
      namespace: default
EOF


if [[ "$CMD" == lustre ]]; then

  # For NNFStorage we need to program the mgsNode value which is under the status section; making it unprogrammable by default.
  # Edit the CRD definition to remove the status section as a subresource.
  echo "$(tput bold)Override NNF subresource $(tput sgr 0)"
  kubectl get crd/nnfstorages.nnf.cray.hpe.com -o json | jq 'del(.spec.versions[0].subresources)' | kubectl apply -f -

  echo "$(tput bold)Installing sample NnfStorage $(tput sgr 0)"
  cat <<-EOF | kubectl apply -f -
  apiVersion: nnf.cray.hpe.com/v1alpha1
  kind: NnfStorage
  metadata:
    name: nnfstorage-sample
  spec:
    fileSystemType: lustre
    allocationSets:
    - name: mgt
      capacity: 1048576
      fileSystemName: sample
      targetType: MGT
      backFs: zfs
      nodes:
      - name: kind-worker
        count: 1
    - name: mdt
      capacity: 1048576
      fileSystemName: sample
      targetType: MDT
      backFs: zfs
      nodes:
      - name: kind-worker2
        count: 1
    - name: ost
      capacity: 1048576
      fileSystemName: sample
      targetType: OST
      backFs: zfs
      nodes:
      - name: kind-worker
        count: 1
      - name: kind-worker2
        count: 1
EOF

fi

if [[ "$CMD" == xfs ]]; then

  echo "$(tput bold)Installing kind-worker Namespaces $(tput sgr 0)"
  cat <<-EOF | kubectl apply -f -
  apiVersion: v1
  kind: Namespace
  metadata:
    name: kind-worker
EOF

  echo "$(tput bold)Installing sample XFS NnfStorage $(tput sgr 0)"
  cat <<-EOF | kubectl apply -f -
  apiVersion: nnf.cray.hpe.com/v1alpha1
  kind: NnfStorage
  metadata:
    name: nnfstorage-sample
  spec:
    fileSystemType: xfs
    allocationSets:
    - name: xfs
      capacity: 1048576
      fileSystemName: sample
      nodes:
      - name: kind-worker
        count: 1
EOF

fi


echo "$(tput bold)Running make deploy to install data movement $(tput sgr 0)"
make deploy

if [[ $(kubectl get rsynctemplates -n nnf-dm-system 2>&1) =~ "No resources found" ]]; then
  echo "$(tput bold)Rsync Template not loaded, retrying deployment $(tput sgr 0)"
  make deploy
fi

if [[ "$CMD" == xfs ]]; then
  echo "$(tput bold)Patching rsync template to disable Lustre File Systems $(tput sgr 0)"
  echo "This will allow the rsync nodes to start - which is otherwise prevented"
  echo "since the Lustre CSI is not loaded"
  kubectl get rsynctemplate/nnf-dm-rsynctemplate -n nnf-dm-system -o json | jq '.spec += {"disableLustreFileSystems": true}' | kubectl apply -f -
fi

