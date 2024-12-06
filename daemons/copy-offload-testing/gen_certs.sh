#!/bin/bash

# Copyright 2024 Hewlett Packard Enterprise Development LP
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

HOST=localhost

CA_DIR="$CERTDIR"/ca
CA_PDIR=$CA_DIR/private
CA_PARAM=$CA_DIR/ca.param
CA_KEY=$CA_PDIR/ca_key.pem

echo "Generate a key using EC algorithm"
mkdir -p "$CA_DIR" && chmod 700 "$CA_DIR"
mkdir -p "$CA_PDIR" && chmod 700 "$CA_PDIR"
openssl genpkey -genparam -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$CA_PARAM"
openssl genpkey -paramfile "$CA_PARAM" -out "$CA_KEY"

SERVER_DIR="$CERTDIR"/server
SERVER_CERT=$SERVER_DIR/server_cert.pem
SERVER_CSR=$SERVER_DIR/server_cert.csr

echo "Generate a self-signed server certificate signing request and certificate"
mkdir -p "$SERVER_DIR" && chmod 700 "$SERVER_DIR"
openssl req -new -key "$CA_KEY" -out "$SERVER_CSR" -subj "/CN=$HOST"
openssl x509 -req -days 1 -in "$SERVER_CSR" -key "$CA_KEY" -out "$SERVER_CERT"

CLIENT_DIR="$CERTDIR"/client
CLIENT_CERT=$CLIENT_DIR/client_cert.pem
CLIENT_CSR=$CLIENT_DIR/client_cert.csr

echo "Generate a client certificate signing request and certificate"
mkdir -p "$CLIENT_DIR" && chmod 700 "$CLIENT_DIR"
openssl req -new -key "$CA_KEY" -out "$CLIENT_CSR" -subj "/CN=$HOST"
openssl x509 -req -days 1 -in "$CLIENT_CSR" -key "$CA_KEY" -out "$CLIENT_CERT"

exit 0
