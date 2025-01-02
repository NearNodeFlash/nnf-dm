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

SRVR_HOST="$2"
SRVR_HOST=${SRVR_HOST:=localhost}

CLIENT_HOST="$3"
CLIENT_HOST=${CLIENT_HOST:=localhost}

CA_DIR="$CERTDIR"/ca
CA_PDIR=$CA_DIR/private
CA_KEY=$CA_PDIR/ca_key.pem

mkdir -p "$CA_DIR" && chmod 700 "$CA_DIR"
mkdir -p "$CA_PDIR" && chmod 700 "$CA_PDIR"

SERVER_DIR="$CERTDIR"/server
SERVER_CERT=$SERVER_DIR/server_cert.pem


mkdir -p "$SERVER_DIR" && chmod 700 "$SERVER_DIR"

openssl req -newkey rsa:2048 -noenc -keyout "$CA_KEY" -x509 -out "$SERVER_CERT" -subj "/CN=$SRVR_HOST"

CLIENT_DIR="$CERTDIR"/client

mkdir -p "$CLIENT_DIR" && chmod 700 "$CLIENT_DIR"


JWT="$CLIENT_DIR"/token

echo "Generate a JWT for the bearer token"
# In a JWT-style token, only the signature is encrypted. Note that a public
# validation service, such as jwt.io, will be able to verify everything except
# the signature. This is because we are signing with the key we made above for
# our self-signed certificate, rather than with a key that goes with a
# certificate from a known certificate authority.
HEADER=$(echo -n '{"alg":"RS256","typ":"JWT"}' | base64 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//)
PAYLOAD=$(echo -n '{"service":"copy-offload-api"}' | base64 | sed s/\+/-/g |sed 's/\//_/g' | sed -E s/=+$//)
echo "hdr+pay: $HEADER.$PAYLOAD"
SIG=$(echo -n "$HEADER.$PAYLOAD" | openssl dgst -sha256 -binary -sign "$CA_KEY" | base64 | tr -d '\n=' | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//)
echo "$HEADER.$PAYLOAD.$SIG" > "$JWT"

exit 0
