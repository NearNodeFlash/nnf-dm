#!/bin/bash

# Undeploy controller from the K8s cluster specified in ~/.kube/config

KUSTOMIZE=$1

kubectl delete -f config/mpi/mpi-operator.yaml

# When the rsync-template CRD gets deleted all related resource are also
# removed, so the delete will always fail. We ignore all errors at our
# own risk.
$KUSTOMIZE build config/default | kubectl delete -f - || true