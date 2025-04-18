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

NAME_PREFIX="gitops-"
DEFAULT_NAME="edentate"
NO_PUSH=
GITOPS_ENV=kind

usage() {
    echo "Usage: $PROG [ARGS] [-u GH_USER] [-n NAME]"
    echo
    echo "  -e GITOPS_ENV   The name of the gitops env. This is the -e arg for"
    echo "                  the gitops tools. Default: '$GITOPS_ENV'"
    echo "  -n NAME         Name of repo. This is appended to the base name."
    echo "                  Default: $DEFAULT_NAME. So the repo will be called"
    echo "                  $NAME_PREFIX$DEFAULT_NAME."
    echo "  -u GH_USER      Github user name. Looks at \$GH_USER environment"
    echo "                  variable for a default. Current: ${GH_USER:-<not set>}."
    echo "  -N              Do no pushes to the repo. For debugging."
}

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

while getopts 'hNe:u:n:' opt; do
case "$opt" in
h) usage
   exit 0;;
e) GITOPS_ENV="$OPTARG" ;;
n) DEFAULT_NAME="$OPTARG" ;;
u) GH_USER="$OPTARG" ;;
N) NO_PUSH=1 ;;
\?|:)
   usage
   exit 1;;
esac
done
shift "$((OPTIND - 1))"

if [[ -z $DEFAULT_NAME ]]; then
    echo "Must supply -n"
    exit 1
elif [[ -z $GH_USER ]]; then
    echo "Must set \$GH_USER environment variable, or supply -u"
    exit 1
fi

set -e
set -o pipefail

export LC_ALL=C
[[ -n $NO_PUSH ]] && NO_PUSH_DBG="echo"

NAME="$NAME_PREFIX$DEFAULT_NAME"
REPO="git@github.com:$GH_USER/$NAME.git"
HREPO="https://github.com/$GH_USER/$NAME.git"
NNF_DEPLOY="https://github.com/NearNodeFlash/nnf-deploy.git"

KUST_RESOURCES=
kustomization_resources() {
    local kust_file="$1"
    KUST_RESOURCES=

    msg "Get resources list from $(dirname "$kust_file")"
    if ! KUST_RESOURCES=$(python3 -c 'import yaml, sys; doc = yaml.safe_load(sys.stdin); _ = [print(resource) for resource in doc["resources"]]' < "$kust_file"); then
        do_fail "Unable to read resources from $kust_file."
    fi
}

touch_missing_yamls() {
    local bootstrap
    local application
    local path
    local resource
    local yaml

    msg "Touch missing yamls"
    for bootstrap in environments/"$GITOPS_ENV"/*bootstrap*
    do
        for application in "$bootstrap"/*.yaml
        do
            [[ $application == */kustomization.yaml ]] && continue
            path=$(grep ' path: ' "$application" 2>/dev/null | awk '{print $2}') || continue
            [[ -z $path ]] && continue
            kustomization_resources "$path/kustomization.yaml"
            for resource in $KUST_RESOURCES; do
                yaml="$path/$resource"
                [[ ! -f "$yaml" ]] && touch "$yaml"
            done
        done
    done
}

remove_storedversions_maint() {
    local bootstrap="environments/$GITOPS_ENV/0-bootstrap1"
    local yaml="$bootstrap/0-nnf-storedversions-maint.yaml"
    local kust_file="$bootstrap/kustomization.yaml"

    msg "Remove nnf-storedversions-maint"
    git mv "$yaml" "$yaml-orig"
    sed -i.bak -e 's/^\(\- 0-nnf-storedversions-maint.yaml\)/#\1/' "$kust_file" && rm "$kust_file.bak"
    git add "$kust_file"
}

remove_storage_version_migrator() {
    local bootstrap="environments/$GITOPS_ENV/0-bootstrap1"
    local yaml="$bootstrap/0-storage-version-migrator.yaml"
    local kust_file="$bootstrap/kustomization.yaml"

    msg "Remove storage-version-migrator"
    git mv "$yaml" "$yaml-orig"
    sed -i.bak -e 's/^\(\- 0-storage-version-migrator.yaml\)/#\1/' "$kust_file" && rm "$kust_file.bak"
    git add "$kust_file"
}

reenable_storedversions_maint() {
    local bootstrap="environments/$GITOPS_ENV/0-bootstrap1"
    local yaml="$bootstrap/0-nnf-storedversions-maint.yaml"
    local kust_file="$bootstrap/kustomization.yaml"

    msg "Re-enable nnf-storedversions-maint"
    git mv "$yaml-orig" "$yaml"
    sed -i.bak -e 's/^#\(\- 0-nnf-storedversions-maint.yaml\)/\1/' "$kust_file" && rm "$kust_file.bak"
    git add "$kust_file"
}

reenable_storage_version_migrator() {
    local bootstrap="environments/$GITOPS_ENV/0-bootstrap1"
    local yaml="$bootstrap/0-storage-version-migrator.yaml"
    local kust_file="$bootstrap/kustomization.yaml"

    msg "Re-enable storage-version-migrator"
    git mv "$yaml-orig" "$yaml"
    sed -i.bak -e 's/^#\(\- 0-storage-version-migrator.yaml\)/\1/' "$kust_file" && rm "$kust_file.bak"
    git add "$kust_file"
}

get_and_unpack_manfest() {
    local tag="$1"
    local tarball="$2"
    local pat

    if [[ $GITOPS_ENV == "kind" ]]; then
        pat="manifests-kind.tar"
    else
        pat="manifests.tar"
    fi

    msg "Get KIND env manifest for $tag"
    gh release download "$tag" -R "$NNF_DEPLOY" -O "$tarball" -p "$pat"
    ./tools/unpack-manifest.py -e "$GITOPS_ENV" -m "$tarball"
    rm -f "$tarball"
}

get_thirdparty() {
    local name="$1"
    local url

    if ! url=$(python3 -c 'import yaml, sys; docs = yaml.safe_load_all(sys.stdin); _ = [print(elem["url"]) for doc in docs for elem in doc["thirdPartyServices"] if elem["name"] == "'"$name"'"]' < config/repositories.yaml); then
        do_fail "Unable to get $name from config/repositories.yaml."
    fi
    if ! wget -O config/"$name.tar" "$url"; then
        do_fail "Unable to pull $url"
    fi
}

unpack_thirdparty() {
    local tar="$1"

    if ! tar xfo "$tar" -C environments/"$GITOPS_ENV"; then
        do_fail "Unable to unpack $tar"
    fi
}

set_branch_in_bootstraps() {
    local branch="$1"
    local bootstrap
    local application

    msg "Set branch name $branch in bootstraps"
    for bootstrap in environments/"$GITOPS_ENV"/*bootstrap*
    do
        for application in "$bootstrap"/*.yaml
        do
            [[ $application == */kustomization.yaml ]] && continue
            sed -i.bak -e 's/^\( *targetRevision: \).*$/\1'"$branch"'/' "$application" && rm "$application.bak"
        done
    done
}

# Create a new empty repo for doing upgrades.
create_new_repo() {
    if [[ -n $NO_PUSH ]]; then
        mkdir "$NAME"
        cd "$NAME"
        git init
    else
        gh repo create "$NAME" -d "Gitops repo for testing upgrades" --private
        gh repo clone "$REPO"
        cd "$NAME"
    fi

    # Hook up your private repo to the ArgoCD boilerplate repo.
    git remote add boilerplate-upstream https://github.com/NearNodeFlash/argocd-boilerplate.git
    git fetch boilerplate-upstream

    # Create two local branches from that ArgoCD boilerplate repo.
    git checkout boilerplate-upstream/main
    git checkout -b main
    $NO_PUSH_DBG git push --set-upstream origin main
    git checkout -b boilerplate-main
    $NO_PUSH_DBG git push --set-upstream origin boilerplate-main

    # Get the SystemConfiguration for the desired environment.
    # Get the repositories.yaml file so we can find the CRD upgrade helpers
    # when we need them outside their normal release.
    git remote add nnf-deploy "$NNF_DEPLOY"
    git fetch nnf-deploy
    git checkout nnf-deploy/master -- config/systemconfiguration-"$GITOPS_ENV".yaml
    git checkout nnf-deploy/master -- config/repositories.yaml
    git remote remove nnf-deploy
    get_thirdparty "storage-version-migrator"
    get_thirdparty "nnf-storedversions-maint"

    git checkout main

    # Create a global lustre LustreFileSystem resource for the KIND environment.
    cat <<EOF > config/global-lustre.yaml
apiVersion: lus.cray.hpe.com/v1beta1
kind: LustreFileSystem
metadata:
  name: foo
  namespace: default
spec:
  name: foo
  mgsNids: 172.31.241.111@tcp
  mountRoot: /lus/foo
  namespaces:
    default:
      modes:
      - ReadWriteMany
EOF

cat <<EOF > config/nnf-sos-kind-rel0.1.11.diff
--- a/environments/kind/nnf-sos/nnf-sos.yaml
+++ b/environments/kind/nnf-sos/nnf-sos.yaml
@@ -1291,8 +1291,8 @@ spec:
         name: mnt-dir
       - hostPath:
           path: /localdisk
+          type: DirectoryOrCreate
         name: localdisk
-        type: DirectoryOrCreate
   updateStrategy:
     rollingUpdate:
       maxUnavailable: 25%
EOF

    git add config
    git commit -s -m 'upgrade config things'

    # Setup python virtual environment.
    python3 -m venv venv
    # shellcheck disable=SC1091
    . venv/bin/activate
    pip install pyyaml

    # Create a new environment for the cluster.
    ./tools/new-env.sh -e "$GITOPS_ENV" -r "$HREPO" -C config/systemconfiguration-"$GITOPS_ENV".yaml -L config/global-lustre.yaml
    git add environments
    git commit -s -m "Create $GITOPS_ENV"

    # Touch up the basic gitops stuff.
    touch_missing_yamls

    git add environments
    git commit -s -m 'touch-ups'
}

# =====================================
# Configure v0.1.11 manifests.
#
configure_v0_1_11_manifests() {
    TAG=v0.1.11
    TARBALL="manifests-$TAG.tar"
    BRANCH="rel-$TAG"
    git checkout main
    git checkout -b "$BRANCH"
    get_and_unpack_manfest "$TAG" "$TARBALL"
    set_branch_in_bootstraps "$BRANCH"

    git mv environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig
    # Remove the patches section.
    # Chop off the end of the file, where we think the patches section lives.
    sed -n '1,/^patches/p' environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig | grep -vE '^patches:' > environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml

    # Roll back the API version in SystemConfiguration.
    sed -i.bak -e 's/v1alpha3/v1alpha2/' environments/"$GITOPS_ENV"/site-config/systemconfiguration.yaml && rm environments/"$GITOPS_ENV"/site-config/systemconfiguration.yaml.bak

    # Fix indentation bug in nnf-sos DaemonSet that affects the KIND
    # environment.
    if [[ "$GITOPS_ENV" == "kind" ]]; then
        patch -p1 < config/nnf-sos-kind-rel0.1.11.diff
    fi

    # The CRD upgrade helpers were not being used with this release. So remove
    # them.
    remove_storage_version_migrator
    remove_storedversions_maint

    git add environments
    git commit -s -m "Release $TAG"

    ./tools/verify-deployment.sh -e "$GITOPS_ENV"
    $NO_PUSH_DBG git push --set-upstream origin "$BRANCH"
    msg "Created branch $BRANCH"
}

# =====================================
# Configure v0.1.12 manifests.
#
configure_v0_1_12_manifests() {
    TAG=v0.1.12
    TARBALL="manifests-$TAG.tar"
    BRANCH="rel-$TAG"
    git checkout main
    git checkout -b "$BRANCH"
    get_and_unpack_manfest "$TAG" "$TARBALL"
    set_branch_in_bootstraps "$BRANCH"

    git mv environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig
    # Remove the patches section.
    # Chop off the end of the file, where we think the patches section lives.
    sed -n '1,/^patches/p' environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig | grep -vE '^patches:' > environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml

    # Roll back the API version in SystemConfiguration.
    sed -i.bak -e 's/v1alpha3/v1alpha2/' environments/"$GITOPS_ENV"/site-config/systemconfiguration.yaml && rm environments/"$GITOPS_ENV"/site-config/systemconfiguration.yaml.bak

    # The CRD upgrade helpers were not being used with this release. So remove
    # them.
    remove_storage_version_migrator
    remove_storedversions_maint

    git add environments
    git commit -s -m "Release $TAG"

    ./tools/verify-deployment.sh -e "$GITOPS_ENV"

    $NO_PUSH_DBG git push --set-upstream origin "$BRANCH"
    msg "Created branch $BRANCH"
}

# =====================================
# Configure v0.1.13 manifests.
#
configure_v0_1_13_manifests() {
    TAG=v0.1.13
    TARBALL="manifests-$TAG.tar"
    BRANCH="rel-$TAG"
    git checkout main
    git checkout -b "$BRANCH"
    get_and_unpack_manfest "$TAG" "$TARBALL"
    set_branch_in_bootstraps "$BRANCH"

    git mv environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig
    # Remove the patches section.
    # Chop off the end of the file, where we think the patches section lives.
    sed -n '1,/^patches/p' environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig | grep -vE '^patches:' > environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml

    # Roll back the API version in SystemConfiguration.
    sed -i.bak -e 's/v1alpha3/v1alpha2/' environments/"$GITOPS_ENV"/site-config/systemconfiguration.yaml && rm environments/"$GITOPS_ENV"/site-config/systemconfiguration.yaml.bak

    # The CRD upgrade helpers were not being used with this release. So remove
    # them.
    remove_storage_version_migrator
    remove_storedversions_maint

    git add environments
    git commit -s -m "Release $TAG"

    ./tools/verify-deployment.sh -e "$GITOPS_ENV"

    $NO_PUSH_DBG git push --set-upstream origin "$BRANCH"
    msg "Created branch $BRANCH"

    # Make a variation that includes storage-version-migrator.
    # This is where we asked the customer to begin using it.
    BRANCH2="$BRANCH-svm"
    reenable_storage_version_migrator
    unpack_thirdparty "config/storage-version-migrator.tar"
    set_branch_in_bootstraps "$BRANCH2"

    git checkout -b "$BRANCH2"
    git add environments
    git commit -s -m "Release $TAG with storage-version-migrator"

    ./tools/verify-deployment.sh -e "$GITOPS_ENV"

    $NO_PUSH_DBG git push --set-upstream origin "$BRANCH2"
    msg "Created branch $BRANCH2 with support for storage-version-migrator"
}

# =====================================
# Configure v0.1.14 manifests.
#
configure_v0_1_14_manifests() {
    TAG=v0.1.14
    TARBALL="manifests-$TAG.tar"
    BRANCH="rel-$TAG"
    git checkout main
    git checkout -b "$BRANCH"
    get_and_unpack_manfest "$TAG" "$TARBALL"
    set_branch_in_bootstraps "$BRANCH"

    git mv environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig
    # Remove the patches section.
    # Chop off the end of the file, where we think the patches section lives.
    sed -n '1,/^patches/p' environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig | grep -vE '^patches:' > environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml

    # Roll back the API version in SystemConfiguration.
    sed -i.bak -e 's/v1alpha3/v1alpha2/' environments/"$GITOPS_ENV"/site-config/systemconfiguration.yaml && rm environments/"$GITOPS_ENV"/site-config/systemconfiguration.yaml.bak

    # The storage-version-migrator is the only CRD upgrade helper in this
    # release. Remove the other one.
    remove_storedversions_maint

    git add environments
    git commit -s -m "Release $TAG"

    ./tools/verify-deployment.sh -e "$GITOPS_ENV"

    $NO_PUSH_DBG git push --set-upstream origin "$BRANCH"
    msg "Created branch $BRANCH"
}

# =====================================
# Configure v0.1.15 manifests.
#
configure_v0_1_15_manifests() {
    TAG=v0.1.15
    TARBALL="manifests-$TAG.tar"
    BRANCH="rel-$TAG"
    git checkout main
    git checkout -b "$BRANCH"
    get_and_unpack_manfest "$TAG" "$TARBALL"
    set_branch_in_bootstraps "$BRANCH"

    git mv environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig
    # Remove the patches section.
    # Chop off the end of the file, where we think the patches section lives.
    sed -n '1,/^patches/p' environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml-orig | grep -vE '^patches:' > environments/"$GITOPS_ENV"/nnf-dm/kustomization.yaml

    # The storage-version-migrator is the only CRD upgrade helper in this
    # release. Remove the other one.
    remove_storedversions_maint

    git add environments
    git commit -s -m "Release $TAG"

    ./tools/verify-deployment.sh -e "$GITOPS_ENV"

    $NO_PUSH_DBG git push --set-upstream origin "$BRANCH"
    msg "Created branch $BRANCH"

    # Make a variation that includes nnf-storedversions-maint.
    # This is where we asked the customer to begin using it.
    BRANCH2="$BRANCH-nsvm"
    reenable_storedversions_maint
    unpack_thirdparty "config/nnf-storedversions-maint.tar"
    set_branch_in_bootstraps "$BRANCH2"

    git checkout -b "$BRANCH2"
    git add environments
    git commit -s -m "Release $TAG with nnf-storedversions-maint"

    ./tools/verify-deployment.sh -e "$GITOPS_ENV"

    $NO_PUSH_DBG git push --set-upstream origin "$BRANCH2"
    msg "Created branch $BRANCH2 with support for nnf-storedversions-maint"
}

create_new_repo
configure_v0_1_11_manifests
configure_v0_1_12_manifests
configure_v0_1_13_manifests
configure_v0_1_14_manifests
configure_v0_1_15_manifests

if [[ -z $NO_PUSH ]]; then
    echo
    msg "Completed configuring the gitops repo."
    echo
    echo "Now you must create a \"Personal access token\" for the repo."
    echo "See: https://github.com/NearNodeFlash/argocd-boilerplate?tab=readme-ov-file#using-with-kind-or-a-private-repo"
fi

