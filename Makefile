# Copyright 2021-2024 Hewlett Packard Enterprise Development LP
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

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
# NOTE: git-version-gen will generate a value for VERSION, unless you override it.

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# cray.hpe.com/nnf-dm-bundle:$VERSION and cray.hpe.com/nnf-dm-catalog:$VERSION.
IMAGE_TAG_BASE ?= ghcr.io/nearnodeflash/nnf-dm
IMAGE_TARGET ?= production

# The NNF-MFU container image to use in NNFContainerProfile resources.
NNFMFU_TAG_BASE ?= ghcr.io/nearnodeflash/nnf-mfu
NNFMFU_VERSION ?= master

CONTAINER_BUILDARGS=--build-arg NNFMFU_TAG_BASE=$(NNFMFU_TAG_BASE) --build-arg NNFMFU_VERSION=$(NNFMFU_VERSION)

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

FAILFAST ?= no
test: manifests generate fmt vet envtest ## Run tests.
	if [[ "${FAILFAST}" == yes ]]; then \
		failfast="-ginkgo.fail-fast"; \
	fi; \
	set -o errexit; \
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path --bin-dir $(LOCALBIN))" go test -v ./... -coverprofile cover.out -ginkgo.v $$failfast

container-unit-test: VERSION ?= $(shell cat .version)
container-unit-test: .version ## Run tests inside a container image
	${CONTAINER_TOOL} build -f Dockerfile --label $(IMAGE_TAG_BASE)-$@:$(VERSION)-$@ -t $(IMAGE_TAG_BASE)-$@:$(VERSION) --target testing $(CONTAINER_BUILDARGS) .
	${CONTAINER_TOOL} run --rm -t --name $@-nnf-dm  $(IMAGE_TAG_BASE)-$@:$(VERSION)

##@ Build
RPM_PLATFORM ?= linux/amd64
RPM_TARGET ?= x86_64
.PHONY: build-daemon-rpm
build-daemon-rpm: RPM_VERSION ?= $(shell ./git-version-gen | sed -e 's/\-.*//')
build-daemon-rpm: $(RPMBIN)
build-daemon-rpm: fmt vet ## Build standalone nnf-dm binary and its rpm
	${CONTAINER_TOOL} build --platform=$(RPM_PLATFORM) --build-arg="RPMTARGET=$(RPM_TARGET)" --build-arg="RPMVERSION=$(RPM_VERSION)" --output=type=local,dest=$(RPMBIN) -f daemons/compute/server/Dockerfile.rpmbuild .

.PHONY: build-daemon-local
build-daemon-local: GOOS = $(shell go env GOOS)
build-daemon-local: GOARCH = $(shell go env GOARCH)
build-daemon-local: build-daemon-with

.PHONY: build-daemon
build-daemon: GOOS ?= linux
build-daemon: GOARCH ?= amd64
build-daemon: build-daemon-with

.PHONY: build-daemon-with
build-daemon-with: RPM_VERSION ?= $(shell ./git-version-gen)
build-daemon-with: PACKAGE = github.com/NearNodeFlash/nnf-dm/daemons/compute/server/version
build-daemon-with: $(LOCALBIN)
build-daemon-with: manifests generate fmt vet ## Build standalone nnf-datamovement daemon
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="-X '$(PACKAGE).version=$(RPM_VERSION)'" -o bin/nnf-dm daemons/compute/server/main.go

build: generate fmt vet ## Build manager binary.
	CGO_ENABLED=0 go build -o bin/manager cmd/main.go

run: manifests generate fmt vet ## Run a controller from your host.
	CGO_ENABLED=0 go run cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: VERSION ?= $(shell cat .version)
docker-build: .version ## Build docker image with the manager.
	${CONTAINER_TOOL} build --target $(IMAGE_TARGET) -t $(IMAGE_TAG_BASE):$(VERSION) $(CONTAINER_BUILDARGS) .

.PHONY: docker-build-debug
docker-build-debug: IMAGE_TAG_BASE := $(IMAGE_TAG_BASE)-debug
docker-build-debug: IMAGE_TARGET := debug
docker-build-debug: docker-build

.PHONY: docker-push
docker-push: VERSION ?= $(shell cat .version)
docker-push: .version ## Push docker image with the manager.
	${CONTAINER_TOOL} push $(IMAGE_TAG_BASE):$(VERSION)

.PHONY: docker-push-debug
docker-push-debug: IMAGE_TAG_BASE := $(IMAGE_TAG_BASE)-debug
docker-push-debug: docker-push

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: VERSION ?= $(shell cat .version)
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --target $(IMAGE_TARGET) --tag $(IMAGE_TAG_BASE):$(VERSION) -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

.PHONY: docker-buildx-debug
docker-buildx-debug: IMAGE_TAG_BASE := $(IMAGE_TAG_BASE)-debug
docker-buildx-debug: IMAGE_TARGET := debug
docker-buildx-debug: docker-buildx

kind-push: VERSION ?= $(shell cat .version)
kind-push: .version ## Push docker image to kind
	# Nnf-dm is used on all nodes. It's on the management node for the
	# nnf-dm-controller-manager deployment, and on the rabbit nodes for
	# the nnf-dm-rsyncnode daemonset that is created by that deployment.
	kind load docker-image $(IMAGE_TAG_BASE):$(VERSION)

kind-push-debug: VERSION ?= $(shell cat .version)
kind-push-debug: IMAGE_TAG_BASE := $(IMAGE_TAG_BASE)-debug
kind-push-debug: kind-push

minikube-push: VERSION ?= $(shell cat .version)
minikube-push: .version
	minikube image load $(IMAGE_TAG_BASE):$(VERSION)

## Deployment

edit-image: VERSION ?= $(shell cat .version)
edit-image: .version
	$(KUSTOMIZE_IMAGE_TAG) config/begin default $(IMAGE_TAG_BASE) $(VERSION) $(NNFMFU_TAG_BASE) $(NNFMFU_VERSION)

deploy: kustomize edit-image ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	./deploy.sh deploy $(KUSTOMIZE) config/begin

undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	./deploy.sh undeploy $(KUSTOMIZE) config/default

# Let .version be phony so that a git update to the workarea can be reflected
# in it each time it's needed.
.PHONY: .version
.version: ## Uses the git-version-gen script to generate a tag version
	./git-version-gen --fallback `git rev-parse HEAD` > .version

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: clean-bin
clean-bin:
	if [[ -d $(LOCALBIN) ]]; then \
	  chmod -R u+w $(LOCALBIN) && rm -rf $(LOCALBIN); \
	fi

## Location to place rpms
RPMBIN ?= $(shell pwd)/rpms
$(RPMBIN):
	mkdir $(RPMBIN)

.PHONY: clean-rpmbin
clean-rpmbin:
	if [[ -d $(RPMBIN) ]]; then \
	  rm -rf $(RPMBIN); \
	fi

## Tool Binaries
KUSTOMIZE_IMAGE_TAG ?= ./hack/make-kustomization2.sh
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v5.1.1
CONTROLLER_TOOLS_VERSION ?= v0.14.0

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(LOCALBIN) ## Download kustomize locally if necessary.
	if [[ ! -s $(LOCALBIN)/kustomize || ! $$($(LOCALBIN)/kustomize version) =~ $(KUSTOMIZE_VERSION) ]]; then \
	  rm -f $(LOCALBIN)/kustomize && \
	  { curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }; \
	fi

.PHONY: controller-gen
controller-gen: $(LOCALBIN) ## Download controller-gen locally if necessary.
	if [[ ! -s $(LOCALBIN)/controller-gen || $$($(LOCALBIN)/controller-gen --version | awk '{print $$2}') != $(CONTROLLER_TOOLS_VERSION) ]]; then \
	  rm -f $(LOCALBIN)/controller-gen && GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION); \
	fi

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240320141353-395cfc7486e6

.PHONY: bundle
bundle: manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

.PHONY: bundle-build
bundle-build: VERSION ?= $(shell cat .version)
bundle-build: BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)
bundle-build: .version ## Build the bundle image.
	${CONTAINER_TOOL} build -f bundle.Dockerfile -t $(BUNDLE_IMG) $(CONTAINER_BUILDARGS) .

.PHONY: bundle-push
bundle-push: VERSION ?= $(shell cat .version)
bundle-push: BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)
bundle-push: .version ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)
