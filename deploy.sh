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

# Deploy controller to the K8s cluster specified in ~/.kube/config.

KUSTOMIZE=$1
IMG=$2

kubectl apply -f config/mpi/mpi-operator.yaml

$(cd config/manager && $KUSTOMIZE edit set image controller=$IMG)

$KUSTOMIZE build config/default | kubectl apply -f - || true

# Sometimes the deployment of the RsyncTemplate occurs to quickly for k8s to digest the CRD
# Retry the deployment if this is the case. It seems to be fast enough where we can just
# turn around and re-deploy; but this may need to move to a polling loop if that goes away.
if [[ $(kubectl get rsynctemplates -n nnf-dm-system 2>&1) =~ "No resources found" ]]; then
    $KUSTOMIZE build config/default | kubectl apply -f -
fi