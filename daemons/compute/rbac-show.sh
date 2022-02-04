#!/bin/bash

RESOURCE_PREFIX="nnf-dm"
NAMESPACE=nnf-dm-system

kubectl get serviceaccount/${RESOURCE_PREFIX}-controller-manager -n ${NAMESPACE} -o yaml
kubectl get clusterrolebindings/${RESOURCE_PREFIX}-manager-rolebinding -o yaml
kubectl get clusterroles/${RESOURCE_PREFIX}-manager-role -o yaml