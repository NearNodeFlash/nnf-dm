#!/bin/bash

# Deploy controller to the K8s cluster specified in ~/.kube/config.

KUSTOMIZE=$1
IMG=$2

kubectl apply -f config/mpi/mpi-operator.yaml

$(cd config/manager && $KUSTOMIZE edit set image controller=$IMG)

$KUSTOMIZE build config/default | kubectl apply -f - || true

# Sometimes the deployment of the RsyncTemplate occurs to quickly for k8s to digest the CRD
# Retry the deployment if this is the case. It seems to be fast enough where we can just
# turn around and re-deploy; but this may need to move to a polling loop if that goes away.
if [[ $(kubectl get rsynctemplates -n nnf-dm-system 2>&1) =~ "No resources found" ]]; then
    $KUSTOMIZE build config/default | kubectl apply -f -
fi