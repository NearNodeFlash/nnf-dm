#!/bin/bash

# Common command aliases for data movement. Source this file to load alias into your bash terminal
# i.e. source ./_aliases.sh

alias dmget="kubectl get datamovements/$(kubectl get datamovements --no-headers -n nnf-dm-system | head -n1 | awk '{print $1}') -n nnf-dm-system -o yaml"
alias dmpod="kubectl describe pod/$(kubectl get pods -n nnf-dm-system --no-headers | grep nnf-dm-controller-manager | awk '{print $1}') -n nnf-dm-system"
alias dmlog="kubectl logs $(kubectl get pods -n nnf-dm-system --no-headers | grep nnf-dm-controller-manager | awk '{print $1}') -n nnf-dm-system  -c manager"

alias mpiget="kubectl get mpijob/$(kubectl get mpijobs -n nnf-dm-system --no-headers | awk '{print $1}') -n nnf-dm-system -o yaml"
alias mpilog="kubectl logs $(kubectl get pods -n mpi-operator --no-headers | awk '{print $1'}) -n mpi-operator"

echo "$(tput bold)DONE! Alias commands available:$(tput sgr 0)"
echo "    dmget - get data movement resource"
echo "    dmpod - describe data movement pod"
echo "    dmlog - get data movement controller logs"
echo "    "
echo "    mpiget - get the mpi job"
echo "    mpilog - get the mpi controller logs"