#!/bin/bash

# Copyright 2021, 2022 Hewlett Packard Enterprise Development LP
# Other additional copyright holders may be indicated within.
#
# The entirety of this work is licensed under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
#
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

usage() {
  cat <<EOF
Run Data Movement commands
Usage: $0 COMMAND [ARGS...]

Commands:
  create              create data movement resource
  delete              delete data movement resource
EOF
}

CMD=${1:-"create"}

NAME=${2:-"datamovement-sample"}
NS="nnf-dm-system"

case $CMD in
  create)
    cat <<-EOF | kubectl apply -f -
      apiVersion: nnf.cray.hpe.com/v1alpha1
      kind: NnfDataMovement
      metadata:
        name: $NAME
        namespace: $NS
      spec:
        userId: 0
        groupId: 0
        source:
          path: /mnt/file.in
          storageInstance:
            kind: NnfStorage
            name: nnfstorage-sample
            namespace: default
        destination:
          path: /mnt/file.out
EOF
      ;;
    delete)
      kubectl delete nnfdatamovement/"$NAME" -n "$NS"
      ;;
    *)
      usage
      exit 1
      ;;
esac
