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

# Common functions for data movement. Source this file to load functions into your bash terminal
# i.e. source ./_aliases.sh
function dmpods   { kubectl get pods -n nnf-dm-system "${@:1}"; }

function dmmget   { kubectl get  -n nnf-dm-system datamovementmanagers --no-headers  | head -n1 | awk '{print $1}'; }
function dmmyaml  { kubectl get  -n nnf-dm-system datamovementmanagers/"$(dmmget)" -o yaml; }
function dmmedit  { kubectl edit -n nnf-dm-system datamovementmanagers/"$(dmmget)"; }
function dmmpod   { kubectl get  -n nnf-dm-system pods --no-headers | grep nnf-dm-manager | awk '{print $1}'; }
function dmmlog   { kubectl logs -n nnf-dm-system "$(dmmpod)" -c manager "${@:1}"; }

function dmdsget  { kubectl get      -n nnf-dm-system daemonsets --no-headers | head -n1 | awk '{print $1}'; }
function dmdsdesc { kubectl describe -n nnf-dm-system daemonsets/"$(dmdsget)"; }
function dmdsyaml { kubectl get      -n nnf-dm-system daemonsets/"$(dmdsget)" -o yaml; }
function dmdsedit { kubectl edit     -n nnf-dm-system daemonsets/"$(dmdsget)"; }

function dmpod    { kubectl get pods --no-headers -n nnf-dm-system | grep nnf-dm-controller-manager | awk '{print $1}'; }
function dmlog    { kubectl logs "$(dmpod)" -n nnf-dm-system -c manager; }
function dmexec   { kubectl exec --stdin --tty pod/"$(dmpod)" -n nnf-dm-system -c manager -- /bin/bash; }

function wkpod    { kubectl get pods --no-headers -n nnf-dm-system | grep nnf-dm-worker | head -n1 | awk '{print $1}'; }
function wkyaml   { kubectl get pod/"$(wkpod)" -n nnf-dm-system -o yaml; }
function wkexec   { kubectl exec --stdin --tty pod/"$(wkpod)" -n nnf-dm-system -c "${1:-manager}" -- /bin/bash; }
function wkdesc   { kubectl describe -n nnf-dm-system pod/"$(wkpod)"; }
function wklog    { kubectl logs "$(wkpod)" -n nnf-dm-system -c "${1:-manager}" "${@:2}"; }
function wkdel    { kubectl delete pod/"$(wkpod)" -n nnf-dm-system; }

function dmslist  { kubectl get nnfdatamovements -A; }
function dmsget   { kubectl get nnfdatamovements -A --no-headers | head -n "$((${1:-0}+1))" | tail -n 1 | awk '{print $2}'; }
function dmsgetns { kubectl get nnfdatamovements -A --no-headers | head -n "$((${1:-0}+1))" | tail -n 1 | awk '{print $1}'; }
function dmsyaml  { (( IDX=${1:-1} )) && kubectl get nnfdatamovements/"$(dmsget $IDX)" -n "$(dmsgetns $IDX)" -o yaml; }

cat <<-EOF

    $(tput bold)Data Movement Manager (dmm) Functions:$(tput sgr 0)
    dmmget              get data movement manager's name
    dmmyaml             get data movement manager's yaml
    dmmpod              get data movement manager's pod
    dmmlog              get data movement manager's log

    $(tput bold)Data Movement Daemon Set (dmds) Functions:$(tput sgr 0)
    dmdsget             get data movement daemonset's name
    dmdsyaml            get data movement daemonset's yaml

EOF