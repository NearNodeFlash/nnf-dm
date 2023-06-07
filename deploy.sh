#!/bin/bash

# Copyright 2021-2023 Hewlett Packard Enterprise Development LP
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

usage() {
    cat <<EOF
Deploy or Undeploy Data Movement
Usage $0 COMMAND KUSTOMIZE [IMG] [NNFMFU_IMG]

Commands:
    deploy              Deploy data movement
    undeploy            Undeploy data movement
EOF
}

CMD=$1
KUSTOMIZE=$2
IMG=$3
NNFMFU_IMG=$4

case $CMD in
deploy)
    (cd config/manager &&
       $KUSTOMIZE edit set image controller="$IMG" &&
       $KUSTOMIZE edit set image nnf-mfu="$NNFMFU_IMG")

    $KUSTOMIZE build config/default | kubectl apply -f - || true

    # Sometimes the deployment of the DataMovementManager occurs too quickly for k8s to digest the CRD
    # Retry the deployment if this is the case. It seems to be fast enough where we can just
    # turn around and re-deploy; but this may need to move to a polling loop if that goes away.
    echo "Waiting for DataMovementManager resource to become ready"
    while :; do
        [[ $(kubectl get datamovementmanager -n nnf-dm-system 2>&1) == "No resources found" ]] && sleep 1 && continue
        $KUSTOMIZE build config/default | kubectl apply -f -
        break
    done
    ;;
undeploy)
    # When the DataMovementManager CRD gets deleted all related resource are also
    # removed, so the delete will always fail. We ignore all errors at our
    # own risk.
    $KUSTOMIZE build config/default | kubectl delete --ignore-not-found -f -
    ;;
*)
    usage
    exit 1
    ;;
esac
