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

CLIENT_HOST="$3"
CLIENT_HOST=${CLIENT_HOST:=localhost}

if ! base64 -w0 < /dev/null 2> /dev/null; then
    BASE64="base64"
else
    BASE64="base64 -w0"
fi

CA_DIR="$CERTDIR"/ca
CA_PDIR=$CA_DIR/private
CA_KEY=$CA_PDIR/ca_key.pem

echo "Generate a key using EC algorithm"
mkdir -p "$CA_DIR" && chmod 700 "$CA_DIR"
mkdir -p "$CA_PDIR" && chmod 700 "$CA_PDIR"

openssl ecparam -name secp521r1 -genkey -noout -out "$CA_KEY"
DER_KEY=$(openssl ec -in "$CA_KEY" -outform DER | $BASE64)

SERVER_DIR="$CERTDIR"/server
SERVER_CERT=$SERVER_DIR/server_cert.pem
SERVER_CSR=$SERVER_DIR/server_cert.csr

echo "Generate a self-signed server certificate signing request and certificate"
mkdir -p "$SERVER_DIR" && chmod 700 "$SERVER_DIR"
openssl req -new -key "$CA_KEY" -out "$SERVER_CSR" -subj "/CN=$SRVR_HOST"
openssl x509 -req -days 1 -in "$SERVER_CSR" -key "$CA_KEY" -out "$SERVER_CERT"

CLIENT_DIR="$CERTDIR"/client
CLIENT_CERT=$CLIENT_DIR/client_cert.pem
CLIENT_CSR=$CLIENT_DIR/client_cert.csr

echo "Generate a client certificate signing request and certificate"
mkdir -p "$CLIENT_DIR" && chmod 700 "$CLIENT_DIR"
openssl req -new -key "$CA_KEY" -out "$CLIENT_CSR" -subj "/CN=$CLIENT_HOST"
openssl x509 -req -days 1 -in "$CLIENT_CSR" -key "$CA_KEY" -out "$CLIENT_CERT"

JWT="$CLIENT_DIR"/token

echo "Generate a JWT for the bearer token"
# In a JWT-style token, only the signature is encrypted. Note that a public
# validation service, such as jwt.io, will be able to verify everything except
# the signature. This is because we are signing with the key we made above for
# our self-signed certificate, rather than with a key that goes with a
# certificate from a known certificate authority.
IAT=$(date +%s)

HEADER=$(echo -n '{"alg":"HS256","typ":"JWT"}' | $BASE64 | sed s/\+/-/ | sed -E s/=+$//)
PAYLOAD=$(echo -n '{"sub":"copy-offload-api", "iat":'"$IAT"'}' | $BASE64 | sed s/\+/-/ | sed -E s/=+$//)
SIG=$(echo -n "$HEADER.$PAYLOAD" | openssl dgst -sha256 -binary -hmac "$DER_KEY" | openssl enc -base64 -A | tr -d '=' | tr -- '+/' '-_')

echo -n "$HEADER.$PAYLOAD.$SIG" > "$JWT"

exit 0
