#!/bin/bash

SERVICE_ACCOUNT=nnf-dm-data-movement
NAMESPACE=nnf-dm-system

kubectl get serviceaccount/${SERVICE_ACCOUNT} -n ${NAMESPACE} -o yaml
kubectl get clusterroles/nnf-dm-data-movement -o yaml
kubectl get clusterrolebindings/nnf-dm-data-movement-rolebinding -o yaml