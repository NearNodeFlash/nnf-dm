#!/bin/bash



# Talking to the copy-offload server while it's running in a pod in KIND.
#
# Begin by storing the client's token and TLS cert in your Mac's /tmp/nnf dir
# so it can be found from inside a kind-worker container:
#
# DW_WORKFLOW_NAME=XXX
#
# CLIENT_TOKEN_SECRET=${DW_WORKFLOW_NAME}-token
# CLIENT_TLS_SECRET=nnf-dm-usercontainer-client-tls
#
# $ kubectl get secrets $CLIENT_TOKEN_SECRET -o yaml  | yq -rM '.data.token' | base64 -d > /tmp/nnf/token
# $ kubectl get secrets $CLIENT_TLS_SECRET -o yaml | yq -rM '.data."tls.crt"' | base64 -d > /tmp/nnf/tls.crt
#
# Get the copy-offload server's pod IP and port:
#
# $ kubectl get pods -o wide
#    (find the copy-offload pod and its IP)
#
# Get the port:
#
# $ kubectl logs <podname> | grep Ready
#    (it's in the INFO Ready message, on the "addr" parameter)
#
# Then go to a kind-worker container and load the token into an env var and
# the copy-offload's IP and port:
#
# $ docker exec -it kind-worker3 bash
# root@kind-worker3:/# export DW_WORKFLOW_TOKEN=$(</mnt/nnf/token)
# root@kind-worker3:/# export DW_WORKFLOW_NAME=XXX
# root@kind-worker3:/# export DW_WORKFLOW_NAMESPACE=default
# root@kind-worker3:/# export NNF_CONTAINER_LAUNCHER=kind-worker2
# root@kind-worker3:/# export NNF_CONTAINER_PORTS=5000
#
# Now you're ready to use curl to send a "hello" message to the server.
#
# From your Mac, tail the server's log:
#
# $ kubectl logs -f <podname>
#
# From the kind-worker container, send the hello:
#
# root@kind-worker3:/# curl -H 'Accepts-version: 1.0' -H "Authorization: Bearer $DW_WORKFLOW_TOKEN" --cacert /mnt/nnf/tls.crt https://$NNF_CONTAINER_LAUNCHER:$NNF_CONTAINER_PORTS/hello
#




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

