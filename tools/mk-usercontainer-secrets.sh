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

SERVER_SECRET_TLS=nnf-dm-usercontainer-server-tls
CLIENT_SECRET_TLS=nnf-dm-usercontainer-client-tls
if kubectl get secret "$SERVER_SECRET_TLS" "$CLIENT_SECRET_TLS" 2> /dev/null; then
    echo "The user-container secrets already exist in the cluster."
    exit 0
fi

CERTDIR=certs
./tools/gen_certs.sh "$CERTDIR" "$SRVR_HOST"

KEY=$CERTDIR/ca/ca_key.pem
SERVER_CERT=$CERTDIR/server/server_cert.pem

kubectl create secret tls $SERVER_SECRET_TLS --cert $SERVER_CERT --key $KEY
# Use an opaque secret for the client's TLS cert because we don't want to
# give the key to the client.
kubectl create secret generic $CLIENT_SECRET_TLS --from-file "tls.crt=$SERVER_CERT"
