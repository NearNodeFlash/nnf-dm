#!/bin/bash

NAME_PREFIX="gitops-"
DEFAULT_NAME="edentate"
GH_USER="${GH_USER:=<unset>}"
NO_PUSH=

usage() {
    echo "Usage: $0 [-N] [-u GH_USER] [-n NAME]"
    echo
    echo "    -n NAME      Name of repo. This is appended to the base name."
    echo "                 Default: $DEFAULT_NAME. So the repo would be"
    echo "                 named: $NAME_PREFIX$DEFAULT_NAME."
    echo "    -u GH_USER   Github user name. Looks at \$GH_USER environment"
    echo "                 variable for a default. Default: $GH_USER."
    echo "    -N           Do no pushes to the repo. For debugging."
}

while getopts 'hNu:n:' opt; do
case "$opt" in
h) usage
   exit 0;;
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

set -x

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

    # Get the SystemConfiguration for a KIND environment.
    git remote add nnf-deploy "$NNF_DEPLOY"
    git fetch nnf-deploy
    git checkout nnf-deploy/master -- config/systemconfiguration-kind.yaml
    git remote remove nnf-deploy

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

    git add config
    git commit -s -m 'upgrade config things'

    # Setup python virtual environment.
    python3 -m venv venv
    # shellcheck disable=SC1091
    . venv/bin/activate
    pip install pyyaml

    # Create a new environment for the cluster.
    ./tools/new-env.sh -e kind -r "$HREPO" -C config/systemconfiguration-kind.yaml -L config/global-lustre.yaml
    git add environments
    git commit -s -m 'Create kind'

    # Touch up the basic gitops stuff.

    touch environments/kind/api-priority-fairness/api-priority-fairness.yaml
    touch environments/kind/early-config/nnflustremgt-configmap.yaml
    touch environments/kind/rbac/flux.yaml
    touch environments/kind/nnf-storedversions-maint/nnf-storedversions-maint.yaml
    touch environments/kind/global-lustre/lustremgts.yaml
    touch environments/kind/site-config/mgt-pool-member-nnfstorageprofile.yaml
    touch environments/kind/nnf-dm/nnf-dm-examples.yaml

    for x in namespace-rbac.yaml trigger.yaml migrator.yaml storage_migration_crd.yaml storage_state_crd.yaml; do
        touch environments/kind/storage-version-migrator/"$x"
    done

    git add environments
    git commit -s -m 'touch-ups'
}

get_and_unpack_manfest() {
    local tag="$1"
    local tarball="$2"

    gh release download "$tag" -R "$NNF_DEPLOY" -O "$tarball" -p manifests.tar
    ./tools/unpack-manifest.py -e kind -m "$tarball"
    rm -f "$tarball"
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

    git mv environments/kind/nnf-dm/kustomization.yaml environments/kind/nnf-dm/kustomization.yaml-orig
    sed -n '1,/^patches/p' environments/kind/nnf-dm/kustomization.yaml-orig | grep -vE '^patches:' > environments/kind/nnf-dm/kustomization.yaml

    sed -i.bak -e 's/v1alpha3/v1alpha2/' environments/kind/site-config/systemconfiguration.yaml && rm environments/kind/site-config/systemconfiguration.yaml.bak

    ./tools/verify-deployment.sh -e kind

    git add environments
    git commit -s -m "Release $TAG"
    $NO_PUSH_DBG git push --set-upstream origin "$BRANCH"
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

    git mv environments/kind/nnf-dm/kustomization.yaml environments/kind/nnf-dm/kustomization.yaml-orig
    sed -n '1,/^patches/p' environments/kind/nnf-dm/kustomization.yaml-orig | grep -vE '^patches:' > environments/kind/nnf-dm/kustomization.yaml

    sed -i.bak -e 's/v1alpha3/v1alpha2/' environments/kind/site-config/systemconfiguration.yaml && rm environments/kind/site-config/systemconfiguration.yaml.bak

    ./tools/verify-deployment.sh -e kind

    git add environments
    git commit -s -m "Release $TAG"
    $NO_PUSH_DBG git push --set-upstream origin "$BRANCH"
}


create_new_repo
configure_v0_1_11_manifests
configure_v0_1_12_manifests
