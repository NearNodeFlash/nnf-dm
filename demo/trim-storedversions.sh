#!/bin/bash

# Copyright 2025 Hewlett Packard Enterprise Development LP
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

NNF_CRDS="dataworkflowservices.github.io nnf.cray.hpe.com lus.cray.hpe.com"

usage() {
    echo "Usage: $0 [-l] [-i CRD]"
    echo
    echo "  -l           List NNF and DWS CRDs."
    echo "  -i CRD       Get version info from a CRD. This must be a"
    echo "               fully-qualified name of a CRD."
}

LIST_CRDS=
GET_VERSION_INFO=

while getopts 'hi:l' opt; do
    case "$opt" in
    i) GET_VERSION_INFO="$OPTARG" ;;
    l) LIST_CRDS=1 ;;
    h) usage; exit 0 ;;
    \?) usage; exit 1 ;;
    esac
done
shift "$((OPTIND - 1))"

set -e
set -o pipefail

list_crds() {
    local crds

    crds=$(echo " $NNF_CRDS" | sed 's/ / -e /g')
    # shellcheck disable=SC2086
    # Sort by group then list only the CRD names.
    kubectl get crds --no-headers -o=custom-columns='GROUP:.spec.group,NAME:.metadata.name' | grep $crds | sort | awk '{print $2}'
}

get_version_info() {
    local out
    if ! out=$(kubectl get crd "$GET_VERSION_INFO" -o json 2>&1); then
        echo "Unable to get CRD $GET_VERSION_INFO:"
        echo "$out"
        exit 1
    fi
    echo "Served API Versions:"
    # shellcheck disable=SC2026
    echo "$out" | jq -rM '.spec.versions[]|select(.served=='true')|.name'
    echo
    echo "Unserved API Versions:"
    # shellcheck disable=SC2026
    echo "$out" | jq -rM '.spec.versions[]|select(.served=='false')|.name'
    echo
    echo "Storage Version:"
    # shellcheck disable=SC2026
    echo "$out" | jq -rM '.spec.versions[]|select(.storage=='true')|.name'
    echo
    echo "StoredVersions:"
    echo "$out" | jq -rM '.status.storedVersions[]|.'
}


# kubectl get crd nnfnodestorages.nnf.cray.hpe.com -o yaml | tail
#    storedVersions:
#    - v1alpha3
#    - v1alpha4
#
# Remove v1alpha3 with a json merge patch:
#
# kubectl patch crd nnfnodestorages.nnf.cray.hpe.com --type=json \
#    --subresource=status --patch \
#    '[{"op": "remove", "path": "/status/storedVersions/0"}]'
#

# Same thing, but using a strategic merge patch:
#
# kubectl patch crd nnfnodeblockstorages.nnf.cray.hpe.com --type=strategic \
#    --subresource=status --patch $'status:\n  storedVersions:\n  - v1alpha4'
#
# That looks like a safer way to do it. It's idempotent, and won't remove
# something that should not have been removed.




if [[ -n $LIST_CRDS ]]; then
    list_crds
    exit 0
elif [[ -n $GET_VERSION_INFO ]]; then
    get_version_info
    exit 0
fi

