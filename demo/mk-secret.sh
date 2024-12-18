#!/bin/bash


set -e
set -o pipefail

CERTDIR=certs

SERVER_CERT=$CERTDIR/server/server_cert.pem
CLIENT_CERT=$CERTDIR/client/client_cert.pem
KEY=$CERTDIR/ca/private/ca_key.pem

SERVER_SECRET=nnf-dm-copy-offload-server
kubectl create secret tls $SERVER_SECRET --cert $SERVER_CERT --key $KEY

if [[ -n $MTLS ]]; then
    CLIENT_SECRET=nnf-dm-copy-offload-client
    kubectl create secret tls $CLIENT_SECRET --cert $CLIENT_CERT --key $KEY
fi
