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

set -e
set -o pipefail

CERTDIR="$1"
if [[ -z $CERTDIR ]]; then
    echo "Must specify cert dir"
    exit 1
fi

SAN_CONF="$2"
if [[ -z $SAN_CONF ]]; then
    echo "Must specify name for SAN conf"
    exit 1
fi

make_rabbit_san_conf()
{
    local NAMES_TXT="$1"
    local SAN_CONF="$2"

    local names_san
    names_san="$NAMES_TXT-san"

    if ! kubectl get systemconfiguration default -o json | jq -rM '.spec.storageNodes[] | select(.type == "Rabbit") | .name' > "$NAMES_TXT"; then
        echo "Unable to collect names of Rabbits"
        exit 1
    fi

    if ! cat -n "$NAMES_TXT" | sed -e 's/^ */DNS./' | awk '{print $1" = "$2}' > "$names_san"; then
        echo "Unable to decorate the Rabbit names"
        exit 1
    fi

    if [[ $(wc -l "$NAMES_TXT" | awk '{print $1}') != $(wc -l "$names_san" | awk '{print $1}') ]]; then
        echo "$NAMES_TXT and $names_san not the same length"
        exit 1
    fi

    if ! echo "[v3_req]
subjectAltName = @alt_names
[alt_names]" > "$SAN_CONF"; then
        echo "Unable to start the SAN conf file"
        exit 1
    fi

    if ! cat "$names_san" >> "$SAN_CONF"; then
        echo "Unable to add Rabbits to SAN conf file"
        exit 1
    fi
}

make_rabbit_san_conf "$CERTDIR/rabbits.txt" "$SAN_CONF"

