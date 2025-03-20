# Copyright 2020-2025 Hewlett Packard Enterprise Development LP
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

# These ARGs must be before the first FROM. This allows them to be valid for
# use in FROM instructions.
ARG NNFMFU_TAG_BASE=ghcr.io/nearnodeflash/nnf-mfu
ARG NNFMFU_VERSION=0.1.6

# Build the manager binary
FROM golang:1.22-alpine AS builder_setup

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/
COPY daemons/copy-offload/ daemons/copy-offload/
COPY daemons/copy-offload-testing/ daemons/copy-offload-testing/
COPY daemons/lib-copy-offload/ daemons/lib-copy-offload/
COPY tools/ tools/
COPY Makefile Makefile

RUN apk add make bash && make clean-bin

###############################################################################
FROM builder_setup AS shellchecker

COPY hack/ hack/

RUN apk add shellcheck && rm -f shellcheck_okay && shellcheck tools/*.sh hack/*.sh daemons/copy-offload-testing/e2e-mocked.sh && touch shellcheck_okay

###############################################################################
FROM builder_setup AS builder

ARG TARGETARCH
ARG TARGETOS

# Build
# the GOARCH has a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

###############################################################################
FROM builder_setup AS copy_offload_builder

ARG TARGETARCH
ARG TARGETOS

# Build
# the GOARCH has a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o nnf-copy-offload daemons/copy-offload/cmd/main.go

###############################################################################
FROM builder_setup AS copy_offload_tester_builder

RUN apk add curl-dev gcc libc-dev && \
    make -C ./daemons/lib-copy-offload tester

###############################################################################
FROM builder AS testing

WORKDIR /workspace
RUN mkdir bin

COPY config/ config/
COPY hack/ hack/
COPY --from=copy_offload_builder /workspace/nnf-copy-offload bin/
COPY --from=copy_offload_tester_builder /workspace/daemons/lib-copy-offload/tester daemons/lib-copy-offload/tester

# Force docker build to run the shellcheck stage.
COPY --from=shellchecker /workspace/shellcheck_okay shellcheck_okay

ENV CGO_ENABLED=0

# These are used by the e2e-mocked test for the copy-offload server.
RUN apk add curl openssl

ENTRYPOINT [ "make", "within-container-unit-test" ]

###############################################################################
FROM $NNFMFU_TAG_BASE:$NNFMFU_VERSION AS production

# The following lines are from the mpiFileUtils (nnf-mfu) Dockerfile;
# do not change them unless you know what it is you are doing
RUN sed -i "s/[ #]\(.*StrictHostKeyChecking \).*/ \1no/g" /etc/ssh/ssh_config \
    && echo "    UserKnownHostsFile /dev/null" >> /etc/ssh/ssh_config

# Copy the executable and execute
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]

# Make it easy to figure out which nnf-mfu was used.
#   docker inspect --format='{{json .Config.Labels}}' image:tag
ARG NNFMFU_TAG_BASE
ARG NNFMFU_VERSION
LABEL nnf-mfu="$NNFMFU_TAG_BASE:$NNFMFU_VERSION"

###############################################################################
# Use the nnf-mfu-debug image as a base
FROM $NNFMFU_TAG_BASE-debug:$NNFMFU_VERSION AS debug

# The following lines are from the mpiFileUtils (nnf-mfu) Dockerfile;
# do not change them unless you know what it is you are doing
RUN sed -i "s/[ #]\(.*StrictHostKeyChecking \).*/ \1no/g" /etc/ssh/ssh_config \
    && echo "    UserKnownHostsFile /dev/null" >> /etc/ssh/ssh_config

# Copy the executable and execute
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]

# Make it easy to figure out which nnf-mfu was used.
#   docker inspect --format='{{json .Config.Labels}}' image:tag
ARG NNFMFU_TAG_BASE
ARG NNFMFU_VERSION
LABEL nnf-mfu="$NNFMFU_TAG_BASE-debug:$NNFMFU_VERSION"

###############################################################################
FROM $NNFMFU_TAG_BASE:$NNFMFU_VERSION AS copy_offload_production

# The following lines are from the mpiFileUtils (nnf-mfu) Dockerfile;
# do not change them unless you know what it is you are doing
RUN sed -i "s/[ #]\(.*StrictHostKeyChecking \).*/ \1no/g" /etc/ssh/ssh_config \
    && echo "    UserKnownHostsFile /dev/null" >> /etc/ssh/ssh_config

# Copy the executable and execute
WORKDIR /
COPY --from=copy_offload_builder /workspace/nnf-copy-offload .

ENTRYPOINT ["/nnf-copy-offload"]

# Make it easy to figure out which nnf-mfu was used.
#   docker inspect --format='{{json .Config.Labels}}' image:tag
ARG NNFMFU_TAG_BASE
ARG NNFMFU_VERSION
LABEL nnf-mfu="$NNFMFU_TAG_BASE:$NNFMFU_VERSION"

