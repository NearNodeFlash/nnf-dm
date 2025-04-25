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

SRVR_HOST="localhost"
CERTDIR="certs"

usage() {
    echo "Usage: $0 [-A] [-S] [-H server-host] [cert-dir]"
    echo
    echo " cert-dir         The directory where the certificate and its key"
    echo "                  are written. The directory must not exist."
    echo "                  The default is \"$CERTDIR\"."
    echo
    echo "  -A              Do not generate the SAN section for the certificate."
    echo "                  The SAN section lists the names of all of the rabbits."
    echo "  -S              Do not create K8s Secret resources for the certificate"
    echo "                  and its signing key."
    echo "  -H server-host  The hostname of the server. Do not use this unless"
    echo "                  you know what you are doing."
    echo "                  The default is $SRVR_HOST."
}

CREATE_SAN=1
CREATE_SECRETS=1

while getopts 'AShH:' opt; do
    case "$opt" in
    A) CREATE_SAN= ;;
    S) CREATE_SECRETS= ;;
    H) SRVR_HOST="$OPTARG" ;;
    h) usage; exit 0 ;;
    \?) usage; exit 1 ;;
    esac
done
shift "$((OPTIND - 1))"

if [[ -n $1 ]]; then
    CERTDIR="$1"
fi
if [[ -e $CERTDIR ]]; then
    echo "The cert dir $CERTDIR already exists"
fi

set -e
set -o pipefail

#
# =============
# =============
#
#       A copy of this is in both the nnf-dm and the argocd-boilerplate repos.
#
#       Update the two copies together.
#
# =============
# =============
#

CA_KEY=
SERVER_CERT=
SERVER_SECRET_TLS=nnf-dm-usercontainer-server-tls
CLIENT_SECRET_TLS=nnf-dm-usercontainer-client-tls

# Create the SAN section for the TLS certificate. This reads the rabbit names
# from the SystemConfiguration resource to populate the SAN block.
make_rabbit_san_conf() {
    local NAMES_TXT="$1"
    local SAN_CONF="$2"
    local names_san="$NAMES_TXT-san"

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

    if ! echo "[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
C = US
[v3_req]
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

# Generate the signing key and TLS certificate.
generate_certs() {
    local CA_DIR="$CERTDIR"/ca
    local RABBIT_SAN_CONF="$CERTDIR/rabbit-san.conf"
    local SERVER_DIR="$CERTDIR"/server
    local SERVER_CSR=$SERVER_DIR/server_cert.csr
    CA_KEY=$CA_DIR/ca_key.pem
    SERVER_CERT=$SERVER_DIR/server_cert.pem

    [[ -e "$CA_KEY" || -e "$SERVER_CERT" ]] && return
    mkdir -p "$CA_DIR"
    chmod 700 "$CA_DIR"

    if [[ -n $CREATE_SAN ]]; then
        make_rabbit_san_conf "$CERTDIR/rabbits.txt" "$RABBIT_SAN_CONF"
    else
        if ! echo "[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
C = US
[v3_req]
subjectAltName = @alt_names
[alt_names]
DNS.1 = $SRVR_HOST
" > "$RABBIT_SAN_CONF"; then
            echo "Unable to write the SAN conf file"
            exit 1
        fi
    fi

    echo "Generate keys using EC algorithm"
    openssl ecparam -name secp521r1 -genkey -noout -out "$CA_KEY"

    echo "Generate a self-signed server certificate signing request and certificate"
    mkdir -p "$SERVER_DIR" && chmod 700 "$SERVER_DIR"
    openssl req -new -key "$CA_KEY" -out "$SERVER_CSR" -subj "/CN=$SRVR_HOST" -config "$RABBIT_SAN_CONF" -extensions 'v3_req'
    openssl x509 -req -days 365 -in "$SERVER_CSR" -key "$CA_KEY" -out "$SERVER_CERT" -extfile "$RABBIT_SAN_CONF" -extensions 'v3_req'
}

# Create the secrets for the certificate and its signing key.
create_secrets() {
    if kubectl get secret "$SERVER_SECRET_TLS" "$CLIENT_SECRET_TLS" 2> /dev/null; then
        echo "The user-container secrets already exist in the cluster."
        exit 0
    fi
    if ! kubectl create secret tls $SERVER_SECRET_TLS --cert "$SERVER_CERT" --key "$CA_KEY"; then
        echo "Unable to create the $SERVER_SECRET_TLS secret."
        exit 1
    fi

    # Use an opaque secret for the client's TLS cert because we don't want to
    # give the key to the client.
    if ! kubectl create secret generic $CLIENT_SECRET_TLS --from-file "tls.crt=$SERVER_CERT"; then
        echo "Unable to create the $CLIENT_SECRET_TLS secret."
        exit 1
    fi
}

generate_certs
if [[ -n $CREATE_SECRETS ]]; then
    create_secrets
fi
exit 0

