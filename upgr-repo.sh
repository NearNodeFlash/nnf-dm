#!/bin/bash

set -e

NAME=gitops-edentate

GH_USER=roehrich-hpe
REPO="git@github.com:$GH_USER/$NAME.git"
HREPO="https://github.com/$GH_USER/$NAME.git"

NNF_DEPLOY="https://github.com/NearNodeFlash/nnf-deploy.git"

set -x

# Create a new empty repo for doing upgrades.

gh repo create "$NAME" -d "Gitops repo for testing upgrades" --private
gh repo clone "$REPO"
cd "$NAME"

# Hook up your private repo to the ArgoCD boilerplate repo.
git remote add boilerplate-upstream https://github.com/NearNodeFlash/argocd-boilerplate.git
git fetch boilerplate-upstream

# Create two local branches from that ArgoCD boilerplate repo.
git checkout boilerplate-upstream/main
git checkout -b main
git push --set-upstream origin main
git checkout -b boilerplate-main
git push --set-upstream origin boilerplate-main

# Get the SystemConfiguration for a KIND environment.
git remote add nnf-deploy "$NNF_DEPLOY"
git fetch nnf-deploy
git checkout nnf-deploy/master -- config/systemconfiguration-kind.yaml
git remote remove nnf-deploy

git checkout main
echo "config/" >> .gitignore
git add .gitignore
git commit -s -m 'ignore config'

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

# Setup python virtual environment.
python3 -m venv venv
. venv/bin/activate
pip install pyyaml

cp ../gitops-kind.git/tools/new-env.sh tools/new-env.sh

# Create a new environment for the cluster.
./tools/new-env.sh -e kind -r "$HREPO" -C config/systemconfiguration-kind.yaml -L config/global-lustre.yaml
git add environments
git commit -s -m 'Create kind'

# =====================================
# Fetch v0.1.11 manifests.
#

TAG=v0.1.11
TARBALL="manifests-$TAG.tar"
BRANCH="rel-$TAG"
git checkout -b "$BRANCH"
gh release download "$TAG" -R "$NNF_DEPLOY" -O "$TARBALL" -p manifests.tar
./tools/unpack-manifest.py -e kind -m "$TARBALL"
rm -f "$TARBALL"

# Now make the newer gitops stuff work with the old v0.1.11.

touch environments/kind/api-priority-fairness/api-priority-fairness.yaml
touch environments/kind/early-config/nnflustremgt-configmap.yaml
touch environments/kind/rbac/flux.yaml
touch environments/kind/nnf-storedversions-maint/nnf-storedversions-maint.yaml
touch environments/kind/global-lustre/lustremgts.yaml
touch environments/kind/site-config/mgt-pool-member-nnfstorageprofile.yaml
touch environments/kind/nnf-dm/nnf-dm-examples.yaml

git mv environments/kind/nnf-dm/kustomization.yaml environments/kind/nnf-dm/kustomization.yaml-orig
sed -n '1,/^patches/p' environments/kind/nnf-dm/kustomization.yaml-orig | grep -vE '^patches:' > environments/kind/nnf-dm/kustomization.yaml

for x in namespace-rbac.yaml trigger.yaml migrator.yaml storage_migration_crd.yaml storage_state_crd.yaml; do
    touch environments/kind/storage-version-migrator/"$x"
done

sed -i.bak -e 's/v1alpha3/v1alpha2/' environments/kind/site-config/systemconfiguration.yaml
rm environments/kind/site-config/systemconfiguration.yaml.bak

./tools/verify-deployment.sh -e kind

git add environments
git commit -s -m "Release $TAG"
git push --set-upstream origin "$BRANCH"

git checkout main
git merge --no-squash "$BRANCH"
git push




