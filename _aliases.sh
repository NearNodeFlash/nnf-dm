#!/bin/bash

shopt -s expand_aliases

# Common command aliases for data movement. Source this file to load alias into your bash terminal
# i.e. source ./_aliases.sh

alias dmget="kubectl get nnfdatamovements --no-headers -n nnf-dm-system | head -n1 | awk '{print \$1}'"
alias dmyaml="kubectl get nnfdatamovement/`dmget` -n nnf-dm-system -o yaml"
alias dmdel="kubectl delete nnfdatamovement/`dmget` -n nnf-dm-system"
alias dmpod="kubectl get pods -n nnf-dm-system --no-headers | grep nnf-dm-controller-manager | awk '{print \$1}'"
alias dmlog="kubectl logs `dmpod` -n nnf-dm-system  -c manager"
alias dmsh="kubectl exec --stdin --tty `dmpod` -n nnf-dm-system -c manager -- /bin/bash"

alias mpiget="kubectl get mpijobs -n nnf-dm-system --no-headers | head -n1 | awk '{print \$1}'"
alias mpiyaml="kubectl get mpijob/`mpiget` -n nnf-dm-system -o yaml"
alias mpidel="kubectl delete mpijob/`mpiget` -n nnf-dm-system"
alias mpilog="kubectl logs `kubectl get pods -n mpi-operator --no-headers | awk '{print \$1}'` -n mpi-operator"

alias wrkget="kubectl get pods -n nnf-dm-system --no-headers | grep mpi-worker | head -n1 | awk '{print \$1}'"
alias wrklog="kubectl logs `wrkget` -n nnf-dm-system"

alias lchrget="kubectl get pods -n nnf-dm-system --no-headers | grep mpi-launcher | head -n1 | awk '{print \$1}'"
alias lchryaml="kubectl get pod/`lchrget` -n nnf-dm-system -o yaml"
alias lchrlog="kubectl logs pod/`lchrget` -n nnf-dm-system"
alias lchrdes="kubectl describe pod/`lchrget` -n nnf-dm-system"
alias lchrsh="kubectl exec --stdin --tty `lchrget` -n nnf-dm-system -- /bin/bash"

alias rtget="kubectl get rsynctemplates -n nnf-dm-system --no-headers | awk '{print \$1}'"
alias rtyaml="kubectl get rsynctemplate/`rtget` -n nnf-dm-system -o yaml"

alias rsyncpod="kubectl get pods -n nnf-dm-system -o wide | grep -w kind-worker | awk '{print \$1}'"
alias rsynclog="kubectl logs pod/`rsyncpod` -n nnf-dm-system"
alias rsyncsh="kubectl exec --stdin --tty `rsyncpod` -n nnf-dm-system -- /bin/bash"

echo "$(tput bold)DONE! Alias commands available: $(tput sgr 0)"
echo "    dmget    - get data movement resource"
echo "    dmyaml   - get data movement resource as yaml"
echo "    dmdel    - delete the data movement resource"
echo "    dmpod    - describe data movement pod"
echo "    dmlog    - get data movement controller logs"
echo "    "
echo "    mpiget   - get the mpi job resource"
echo "    mpiyaml  - get the mpi job resource as yaml"
echo "    mpidel   - delete the mpi job"
echo "    mpilog   - get the mpi controller logs"
echo "    "
echo "    wrkget   - get the mpi-worker pod (1 of N)"
echo "    wrklog   - get the mpi-worker logs for pod 1 (of N)"
echo "    "
echo "    lchrget  - get the mpi-launcher resource"
echo "    lchryaml - get the mpi-launcher resource as yaml"
echo "    lchrlog  - get the mpi-launcher log"
echo "    lchrdes  - describe the mpi-launcher pod"
echo "    lchrsh   - execute a shell prompt to the mpi-launcher"
echo "    "
echo "    rtget    - get the rsync template resource"
echo "    rtyaml   - get the rsync template resource as yaml"
echo "    "
echo "    rsyncsh  - shell into kind-worker rsync pod"