#!/bin/bash


SERVICE_ACCOUNT=nnf-dm-data-movement
NAMESPACE=nnf-dm-system

echo "Obtaining token and secret for service account ${SERVICE_ACCOUNT}..."
SECRET=$(kubectl get serviceaccount ${SERVICE_ACCOUNT} -n ${NAMESPACE} -o json | jq -Mr '.secrets[].name | select(contains("token"))')
TOKEN=$(kubectl get secret ${SECRET} -n ${NAMESPACE} -o json | jq -Mr '.data.token' | base64 -D)

echo "Trying to locate kind configuration..."
APISERVER=$(kind get kubeconfig | grep server | awk '{print $2}')

echo "Testing server ${APISERVER}..."
curl -s ${APISERVER}/apis/nnf.cray.hpe.com/v1alpha1/nnfdatamovements --header "Authorization: Bearer $TOKEN" --cacert ./ca.crt 