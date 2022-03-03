#!/bin/bash
CMD=$1

if [[ "$CMD" == "deploy-dp" ]]; then
    kubectl config view | grep current-context
    
    # Install the prerequisite CRDs
    if [[ -z $(kubectl get crds | grep lustrefilesystems.cray.hpe.com) ]]; then
        kubectl apply -f vendor/github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/config/crd/bases/cray.hpe.com_lustrefilesystems.yaml
    fi

    if [[ -z $(kubectl get crds | grep mpijobs.kubeflow.org) ]]; then
        kubectl apply -f config/mpi/mpi-operator.yaml
    fi
    
    source ./setDevVersion.sh
    [[ -z $VERSION ]] && echo "VERSION not set... Aborting" && exit 1

    make deploy
fi