#!/bin/bash

# Copyright 2024-2025 Hewlett Packard Enterprise Development LP
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

SRVR_HOST="$2"
SRVR_HOST=${SRVR_HOST:=localhost}

CA_DIR="$CERTDIR"/ca
CA_KEY=$CA_DIR/ca_key.pem

RABBIT_SAN_CONF="$CERTDIR/rabbit-san.conf"

mkdir -p "$CA_DIR" && chmod 700 "$CA_DIR"

if [[ -z $SKIP_SAN ]]; then
    ./tools/mk-rabbit-san.sh "$CERTDIR" "$RABBIT_SAN_CONF" || exit 1
else
    if ! echo "[v3_req]
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

SERVER_DIR="$CERTDIR"/server
SERVER_CERT=$SERVER_DIR/server_cert.pem
SERVER_CSR=$SERVER_DIR/server_cert.csr

echo "Generate a self-signed server certificate signing request and certificate"
mkdir -p "$SERVER_DIR" && chmod 700 "$SERVER_DIR"
openssl req -new -key "$CA_KEY" -out "$SERVER_CSR" -subj "/CN=$SRVR_HOST" -config "$RABBIT_SAN_CONF" -extensions 'v3_req'
openssl x509 -req -days 365 -in "$SERVER_CSR" -key "$CA_KEY" -out "$SERVER_CERT" -extfile "$RABBIT_SAN_CONF" -extensions 'v3_req'

exit 0
