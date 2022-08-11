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


CMD=$1
if [[ "$CMD" == "kind-reset" ]]; then
  kind delete cluster
fi

echo "$(tput bold)Creating temporary source file for data movement $(tput sgr 0)"
mkdir -p /tmp/nnf && dd if=/dev/zero of=/tmp/nnf/file.in bs=128 count=0 seek=$[1024 * 1024]

echo "$(tput bold)Creating kind cluster with two worker nodes and /nnf mount $(tput sgr 0)"
kind create cluster --wait 60s --image=kindest/node:v1.22.5 --config kind-config.yaml

kubectl taint node kind-control-plane node-role.kubernetes.io/master:NoSchedule-
kubectl label node kind-control-plane cray.nnf.manager=true
kubectl label node kind-control-plane cray.wlm.manager=true

# Label the kind-workers as rabbit nodes for the NLCMs.
NODES=$(kubectl get nodes --no-headers | grep --invert-match "control-plane" | awk '{print $1}')
for NODE in $NODES; do
    kubectl label node "$NODE" cray.nnf.node=true
    kubectl label node "$NODE" cray.nnf.x-name="$NODE"
done

kubectl taint nodes $(echo $NODES | paste -d" " -s -) cray.nnf.node=true:NoSchedule

certver="v1.7.0"
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/"$certver"/cert-manager.yaml

echo "$(tput bold)DONE!$(tput sgr 0)"
echo "You should now ensure any necessary images are loaded in kind"
echo "and then run _setup-cluster.sh [lustre, xfs] to setup data-movement."
