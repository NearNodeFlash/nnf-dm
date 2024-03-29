#!/bin/bash

# Copyright 2021-2024 Hewlett Packard Enterprise Development LP
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

set -e
set -o pipefail

usage() {
    cat <<EOF
Deploy or Undeploy Data Movement
Usage $0 COMMAND KUSTOMIZE <OVERLAY_DIR>

Commands:
    deploy              Deploy data movement
    undeploy            Undeploy data movement
EOF
}

CMD=$1
KUSTOMIZE=$2
OVERLAY_DIR=$3

case $CMD in
deploy)
    $KUSTOMIZE build "$OVERLAY_DIR" | kubectl apply -f - || true

    # Sometimes the deployment of the NnfDataMovementManager occurs too quickly for k8s to digest the CRD
    # Retry the deployment if this is the case. It seems to be fast enough where we can just
    # turn around and re-deploy; but this may need to move to a polling loop if that goes away.
    echo "Waiting for NnfDataMovementManager resource to become ready"
    while :; do
        [[ $(kubectl get nnfdatamovementmanager -n nnf-dm-system 2>&1) == "No resources found" ]] && sleep 1 && continue
        $KUSTOMIZE build "$OVERLAY_DIR" | kubectl apply -f -
        break
    done

    # Deploy the ServiceMonitor resource if its CRD is found. The CRD would
    # have been installed by a metrics service such as Prometheus.
    if kubectl get crd servicemonitors.monitoring.coreos.com > /dev/null 2>&1; then
        $KUSTOMIZE build config/prometheus | kubectl apply -f-
    fi
    ;;
undeploy)
    if kubectl get crd servicemonitors.monitoring.coreos.com > /dev/null 2>&1; then
        $KUSTOMIZE build config/prometheus | kubectl delete --ignore-not-found -f-
    fi
    # When the NnfDataMovementManager CRD gets deleted all related resource are also
    # removed, so the delete will always fail. We ignore all errors at our
    # own risk.
    # Do not touch the namespace resource when deleting this service.
    # Wishing for yq(1)...
    $KUSTOMIZE build "$OVERLAY_DIR" | python3 -c 'import yaml, sys; all_docs = yaml.safe_load_all(sys.stdin); less_docs=[doc for doc in all_docs if doc["kind"] != "Namespace"]; print(yaml.dump_all(less_docs))' |  kubectl delete --ignore-not-found -f -
    ;;
*)
    usage
    exit 1
    ;;
esac
