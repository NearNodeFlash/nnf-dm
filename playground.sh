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

if [[ "$CMD" == "deploy-dp" ]]; then
    kubectl config view | grep current-context

    # Install the prerequisite CRDs
    if [[ -z $(kubectl get crds | grep lustrefilesystems.cray.hpe.com) ]]; then
        kubectl apply -f vendor/github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/config/crd/bases/cray.hpe.com_lustrefilesystems.yaml
    fi

    if [[ -z $(kubectl get crds | grep mpijobs.kubeflow.org) ]]; then
        kubectl apply -f config/mpi/mpi-operator.yaml
    fi

    source ./setDevVersion.sh
    [[ -z $VERSION ]] && echo "VERSION not set... Aborting" && exit 1

    make deploy
fi