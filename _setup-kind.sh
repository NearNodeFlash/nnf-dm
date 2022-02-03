#!/bin/bash

CMD=$1
if [[ "$CMD" == "kind-reset" ]]; then
  kind delete cluster
fi

echo "$(tput bold)Creating temporary source file for data movement $(tput sgr 0)"
mkdir -p /tmp/nnf && dd if=/dev/zero of=/tmp/nnf/file.in bs=128 count=0 seek=$[1024 * 1024]

echo "$(tput bold)Creating kind cluster with two worker nodes and /nnf mount $(tput sgr 0)"
kind create cluster --wait 60s --image=kindest/node:v1.20.0 --config kind-config.yaml

kubectl taint node kind-control-plane node-role.kubernetes.io/master:NoSchedule-
kubectl label node kind-control-plane cray.nnf.manager=true
kubectl label node kind-control-plane cray.wlm.manager=true

# Label the kind-workers as rabbit nodes for the NLCMs.
for NODE in $(kubectl get nodes --no-headers | grep --invert-match "control-plane" | awk '{print $1}'); do
    kubectl label node "$NODE" cray.nnf.node=true
    kubectl label node "$NODE" cray.nnf.x-name="$NODE"
done

kubectl taint nodes $(kubectl get nodes --no-headers -o custom-columns=:metadata.name | grep -v control-plane | paste -d" " -s -) cray.nnf.node=true:NoSchedule

echo "$(tput bold)DONE!$(tput sgr 0)"
echo "You should now ensure any necessary images are loaded in kind"
echo "and then run _setup-cluster.sh [lustre, xfs] to setup data-movement."