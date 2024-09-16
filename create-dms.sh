#!/bin/bash

# This script is used to create NnfDataMovement resources via kubectl apply. This mimics the Copy
# Offload API and is useful for testing. It assumes that a workflow is in the PreRun state. You can
# create a large number of resources simulteanously and each request is sequentially numbered. This
# is a script used for development.

# User must exist on the compute node
USER=
COMPUTE=rabbit-compute-5

WORKFLOW=
PROFILE=default

# Options
CREATE=""
DELETE_ALL=""
DELETE_SUCCESS=""
GET_FAILED=""
NUM_REQUESTS=1
WAIT=""

OPTSTRING="CDdgn:p:w"
while getopts "${OPTSTRING}" opt; do
  case ${opt} in

  C)
    CREATE="true"
    ;;
  D)
    DELETE_ALL="true"
    ;;
  d)
    DELETE_SUCCESS="true"
    ;;
  g)
    GET_FAILED="true"
    ;;
  n)
    NUM_REQUESTS=${OPTARG}
    ;;
  p)
    PROFILE=${OPTARG}
    ;;
  w)
    WAIT="true"
    ;;
  *)
    echo "invalid option"
    exit 1
    ;;
  esac
done

shift $((OPTIND - 1))

if [[ "$#" -ne 2 ]]; then
  echo "Usage: $0 [--options] <user> <workflow>"
  exit 1
fi

# Assign positional arguments to variables
USER=$1
WORKFLOW=$2

# testfile needs to present in this directory - create this manually (with the right perms too)
# might need to have i number of testfiles
# source=3d32f595-3bbd-4cef-a17c-e10c123c02bc-0
wf_yaml=$(kubectl get workflow "${WORKFLOW}" -oyaml)
source=$(echo "${wf_yaml}" -oyaml | yq '.metadata.uid')-0
testfile=/mnt/nnf/${source}/testfile

if [[ -n "${CREATE}" ]]; then
  # create a file to move
  echo -n "Creating ${testfile} on ${COMPUTE}..."
  set -e
  ssh ${COMPUTE} "fallocate -x -l 100M ${testfile}"
  ssh ${COMPUTE} "chown ${USER}:users ${testfile}"
  set +e
  echo "done"

  # creat requests
  echo -n "Creating ${NUM_REQUESTS} requests..."
  for ((i = 1; i <= NUM_REQUESTS; i++)); do
    echo "
apiVersion: nnf.cray.hpe.com/v1alpha2
kind: NnfDataMovement
metadata:
  name: ${WORKFLOW}-${i}
  namespace: nnf-dm-system
spec:
  cancel: false
  destination:
    path: /lus/global/${USER}/testdir
    storageReference:
      kind: LustreFileSystem
      name: lushtx
      namespace: default
  groupId: 100
  profileReference:
      kind: NnfDataMovementProfile
      name: ${PROFILE}
      namespace: nnf-system
  source:
    path: /mnt/nnf/${source}/testfile
    storageReference:
      kind: NnfStorage
      name: ${WORKFLOW}-0
      namespace: default
  userId: 1060
  " | kubectl apply -f - &
  done
  echo "done"

  # wait for all of them to complete data movement
  if [[ -n "${WAIT}" ]]; then
    echo -n "Waiting for completion of all data movement resources..."
    while :; do
      num_done=$(kubectl get nnfdatamovements -A --no-headers | grep -c Success || true)
      if [[ "${num_done}" -eq "${NUM_REQUESTS}" ]]; then
        break
      fi
    done
    echo "done"
  fi

fi

if [[ -n "${DELETE_ALL}" ]]; then
  echo -n "Deleting all nnfdatamovements..."
  kubectl get nnfdatamovements -n nnf-dm-system --no-headers | awk '{print $1}' | xargs -n 1 -P 0 kubectl delete -n nnf-dm-system nnfdatamovement || true
  echo "done"
elif [[ -n "${DELETE_SUCCESS}" ]]; then
  echo -n "Deleting Successful nnfdatamovements..."
  kubectl get nnfdatamovements -n nnf-dm-system --no-headers | grep Success | awk '{print $1}' | xargs -n 1 -P 0 kubectl delete -n nnf-dm-system nnfdatamovement || true
  echo "done"
fi

if [[ -n "${GET_FAILED}" ]]; then
  kubectl get nnfdatamovements -A -oyaml | yq '.items[].status' || true
fi
