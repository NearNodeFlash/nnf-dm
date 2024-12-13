#!/bin/bash


# This is kind-worker3 sub'ing as a compute, sending a hello to kind-worker2:
#
# root@kind-worker3:/# curl 172.18.0.2:8080/hello


# This is kind-worker3 sub'ing as a compute, sending a copy-offload request
# to kind-worker2:
#
# root@kind-worker3:/# curl -v -X POST -d @/mnt/nnf/offload-req.yaml 172.18.0.3:8080/trial


set -e
set -o pipefail

demo="$1"
if [[ -z $demo ]]; then
  echo "Need demo name"
  exit 1
fi

if $(kubectl config current-context | grep -q kind-kind); then
  USE_KIND=1
fi

echo "Using $demo"
if [[ -n $USE_KIND ]]; then
  echo "in KIND"
  SUFFIX="-kind"
fi
echo

COMPUTES_PATCH="demo/allocation-computes$SUFFIX.yaml"
SERVERS_PATCH="demo/allocation-servers$SUFFIX.yaml"

set -x

#bin/kustomize build config/copy-offload | kubectl apply -f-
kubectl apply -f demo/demo-datamovementprofile.yaml

kubectl apply -f "$demo"
wfname=$(yq -M 'select(.kind=="Workflow")|.metadata.name' "$demo")

kubectl wait workflow "$wfname" --timeout=180s --for jsonpath='{.status.status}=Completed'

kubectl patch --type merge --patch-file="$COMPUTES_PATCH" computes "$wfname"
kubectl patch --type merge --patch-file="$SERVERS_PATCH" servers "$wfname-0"

kubectl patch --type merge workflow "$wfname" --patch '{"spec": {"desiredState": "Setup"}}'
kubectl wait workflow "$wfname" --timeout=180s --for jsonpath='{.status.state}=Setup'
kubectl wait workflow "$wfname" --timeout=180s --for jsonpath='{.status.status}=Completed'

kubectl patch --type merge workflow "$wfname" --patch '{"spec": {"desiredState": "DataIn"}}'
kubectl wait workflow "$wfname" --timeout=180s --for jsonpath='{.status.state}=DataIn'
kubectl wait workflow "$wfname" --timeout=180s --for jsonpath='{.status.status}=Completed'

kubectl patch --type merge workflow "$wfname" --patch '{"spec": {"desiredState": "PreRun"}}'
kubectl wait workflow "$wfname" --timeout=180s --for jsonpath='{.status.state}=PreRun'
kubectl wait workflow "$wfname" --timeout=300s --for jsonpath='{.status.status}=Completed'

