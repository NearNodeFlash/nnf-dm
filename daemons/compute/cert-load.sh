#!/bin/bash

SERVICE_ACCOUNT=nnf-dm-controller-manager
NAMESPACE=nnf-dm-system

SECRET=$(kubectl get serviceaccount ${SERVICE_ACCOUNT} -n ${NAMESPACE} -o json | jq -Mr '.secrets[].name | select(contains("token"))')
TOKEN=$(kubectl get secret ${SECRET} -n ${NAMESPACE} -o json | jq -Mr '.data.token' | base64 -D)
kubectl get secret ${SECRET} -n ${NAMESPACE} -o json | jq -Mr '.data.token' | base64 -D > ./token
kubectl get secret ${SECRET} -n ${NAMESPACE} -o json | jq -Mr '.data["ca.crt"]' | base64 -D > ./ca.crt