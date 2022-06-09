# Copyright 2020, 2021, 2022 Hewlett Packard Enterprise Development LP
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

# Build the manager binary
FROM golang:1.17-alpine as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

###############################################################################
FROM builder as testing

WORKDIR /workspace

RUN apk add rsync curl tar

# These two steps follow the Kubebuilder envtest playbook https://book.kubebuilder.io/reference/envtest.html
ARG K8S_VERSION=1.23.3
RUN curl -sSLo envtest-bins.tar.gz "https://go.kubebuilder.io/test-tools/${K8S_VERSION}/$(go env GOOS)/$(go env GOARCH)" \
    && mkdir /usr/local/kubebuilder \
    && tar -C /usr/local/kubebuilder --strip-components=1 -zvxf envtest-bins.tar.gz


COPY config/ config/
COPY initiateContainerTest.sh .

ENTRYPOINT [ "sh", "initiateContainerTest.sh" ]

###############################################################################
FROM alpine:latest

RUN apk add rsync

WORKDIR /
COPY --from=builder /workspace/manager .
#USER 65532:65532

ENTRYPOINT ["/manager"]
