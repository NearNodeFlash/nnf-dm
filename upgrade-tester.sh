#!/bin/bash

# Copyright 2025 Hewlett Packard Enterprise Development LP
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

PROG=$(basename "$0")

BOLD=$(tput bold)
NORMAL=$(tput sgr0)

do_fail() {
    echo "${BOLD}$*$NORMAL"
    exit 1
}

msg(){
    local msg="$1"

    echo "${BOLD}${msg}${NORMAL}"
}

GITOPS_ENV=kind

usage() {
    echo "Usage: $PROG [ARGS] <release0 <release1 <releaseN>>>"
    echo
    echo "  -e GITOPS_ENV   The name of the gitops env. This is the -e arg for"
    echo "                  the gitops tools. Default: '$GITOPS_ENV'"
    echo "  -n              Dry run."
    echo
}

while getopts 'he:n' opt; do
case "$opt" in
    e) GITOPS_ENV="$OPTARG" ;;
    n) DRYRUN=1 ;;
    h) usage
       exit 0;;
    :) echo "Option -$OPTARG requires an argument."
       exit 1;;
    ?)
       echo "Invalid option -$OPTARG."
       exit 1;;
esac
done
shift "$((OPTIND - 1))"

if ! which python3 >/dev/null 2>&1; then
    echo "Unable to find python3 in PATH"
    exit 1
elif ! python3 -c 'import yaml' 2>/dev/null; then
    echo "Unable to find PyYAML"
    exit 1
fi

export LC_ALL=C

set -e
set -o pipefail


verify_gitops_git_repo() {
    local releases=("$@")

    echo "Checking for git repo"
    git status > /dev/null 2>&1 || do_fail "Not in a git repo."
    echo "Checking for gitops repo"
    [[ -d "environments/$GITOPS_ENV/0-bootstrap0" ]] || do_fail "Repo does not look like an ArgoCD gitops repo."
    [[ -x tools/deploy-env.sh ]] || do_fail "Repo does not contain the gitops deploy-env.sh tool."

    for release_ver in "${releases[@]}"; do
        echo "Checking for branch $release_ver"
        git branch -a | grep -E '[ /]'"$release_ver"'$' || do_fail "Repo does not contain a branch named $release_ver."
    done
}

verify_kind_cluster() {
    echo "Checking for KIND cluster"
    if [[ -z $DRYRUN ]]; then
        command -v kind >/dev/null 2>&1 || do_fail "Kind is not installed."
        kind get clusters | grep -q kind || do_fail "No Kind cluster is running."
    fi
}

verify_argocd() {
    echo "Checking for ArgoCD"
    if [[ -z $DRYRUN ]]; then
        kubectl wait deploy -n argocd --timeout=180s nnf-argocd-server --for jsonpath='{.status.availableReplicas}=1' || do_fail "ArgoCD is not running in cluster."
    fi

    echo "Checking for ArgoCD CLI"
    export ARGOCD_OPTS='--port-forward --port-forward-namespace argocd'
    if [[ -z $DRYRUN ]]; then
        command -v argocd >/dev/null 2>&1 || do_fail "ArgoCD CLI is not installed."
        argocd account get-user-info 2> /dev/null | grep -q 'Logged In: true' || do_fail "ArgoCD CLI is not logged in."
    fi
}

verify_argocd_repo_hookup() {
    local repo
    local repo_list
    local want_repo

    repo_list=$(argocd repo list -o json | jq -rM '.[]|.repo')
    if ! want_repo=$(grep repoURL: environments/"$GITOPS_ENV"/2-bootstrap0/1-dws.yaml | awk '{print $2}'); then
        do_fail "Unable to get repoURL from bootstrap for 1-dws.yaml."
    fi
    for repo in $repo_list; do
        if [[ $repo == "$want_repo" ]]; then
            return
        fi
    done
    msg "ArgoCD does not have access to the gitops repo."
    msg "Repo it needs: $want_repo"
    msg "Current repos it can access:"
    argocd repo list
    exit 1
}

verify_bootstraps_match_branch() {
    local branch="$1"
    local want

    case "$branch" in
    main|master) want="HEAD" ;;
    *)           want="$branch" ;;
    esac

    if [[ $(grep -E '^ *targetRevision: ' environments/"$GITOPS_ENV"/*bootstrap*/*.yaml | grep -cvE ' '"$want"'$') != 0 ]]; then
        msg "Branch $branch has bootstraps that do not match the branch."
        grep -E '^ *targetRevision: ' environments/"$GITOPS_ENV"/*bootstrap*/*.yaml | grep -vE " $want$"
        exit 1
    fi
}

set_dws_webhook_service_type() {
    local release_ver="$1"
    local svc_type=

    # The NNF v0.1.15 release changed the type of Service/dws-webhook-service 
    # from LoadBalancer to ClusterIP.
    # This will set svc_type to "LoadBalancer" or nothing.

    if ! svc_type=$(python3 -c 'import yaml, sys; docs = yaml.safe_load_all(sys.stdin); _ = [print("LoadBalancer") for doc in docs if doc is not None and doc["kind"] == "Service" and doc["metadata"]["name"] == "dws-webhook-service" and "type" in doc["spec"] and doc["spec"]["type"] == "LoadBalancer"]' < environments/kind/dws/dws.yaml) ; then
        do_fail "Unable to determine type of dws-webhook-service in $release_ver."
    fi
    DWS_WEBHOOK_SVC_TYPE="$svc_type"
}

wait_until_stable() {
    local release_ver="$1"
    local stable=
    local stable_dws=
    local dws_stable_pat=

    case $DWS_WEBHOOK_SVC_TYPE in
    LoadBalancer)
        dws_stable_pat="Synced *Progressing"
        msg "Expect DWS to settle with Synced/Progressing"
        ;;
    *)
        dws_stable_pat="Synced *Healthy"
        ;;
    esac

    msg "Waiting until $release_ver settles"
    sleep 10
    while :; do
        # This is a turbulent time in ArgoCD, so we re-check everything on each
        # pass.
        stable=
        stable_dws=

        if [[ $(kubectl get applications -n argocd --no-headers 1-dws | grep -cvE "$dws_stable_pat") == 0 ]]; then
            stable_dws=1
        fi

        # Check all non-dws services.
        if [[ $(kubectl get applications -n argocd --no-headers | grep -v 1-dws | grep -cvE 'Synced *Healthy') == 0 ]]; then
            stable=1
        fi

        [[ -n $stable && -n $stable_dws ]] && break

        echo -n "."
        sleep 10
    done

    echo
    msg "Apps:"
    kubectl get applications -n argocd
}

verify_gitops_git_repo "$@"
verify_kind_cluster
verify_argocd
verify_argocd_repo_hookup

echo
date_begin=$(date)
msg "$date_begin"
echo
for release_ver in "$@"; do
    DWS_WEBHOOK_SVC_TYPE=

    msg "Checkout $release_ver"
    git checkout "$release_ver" || do_fail "Unable to checkout $release_ver."
    verify_bootstraps_match_branch "$release_ver"

    set_dws_webhook_service_type "$release_ver"

    # The bootstraps have changed, so we must re-deploy them. ArgoCD does not
    # monitor the bootstraps.
    rel_begin=$(date)
    msg "Release $release_ver begin: $rel_begin"
    msg "Deploy $release_ver"
    ./tools/deploy-env.sh -e "$GITOPS_ENV"
    wait_until_stable "$release_ver"
    msg "Release $release_ver begin: $rel_begin"
    msg "Release $release_ver end  : $(date)"
    echo
done
echo
echo "Beginning timestamp: $date_begin"
echo "Ending timestamp   : $(date)"

