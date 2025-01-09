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

SRVR_HOST="$1"
SRVR_HOST=${SRVR_HOST:=localhost}

CERTDIR=daemons/copy-offload/certs
./daemons/copy-offload/tools/gen_certs.sh "$CERTDIR" "$SRVR_HOST"

KEY=$CERTDIR/ca/private/ca_key.pem
SERVER_CERT=$CERTDIR/server/server_cert.pem
TOKEN=$CERTDIR/client/token

SERVER_SECRET=nnf-dm-copy-offload-server
kubectl create secret tls $SERVER_SECRET --cert $SERVER_CERT --key $KEY

TOKEN_SECRET=nnf-dm-copy-offload-client
kubectl create secret generic $TOKEN_SECRET --from-file "tls.crt"="$SERVER_CERT" --from-file token="$TOKEN"
