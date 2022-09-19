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
FROM golang:1.19-alpine as builder

WORKDIR /workspace
ENV GOPATH=/go

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

# Build debug
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install -ldflags "-s -w -extldflags '-static'" github.com/go-delve/delve/cmd/dlv@latest
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags "all=-N -l" -a -o manager-debug main.go

###############################################################################
FROM builder as testing

WORKDIR /workspace

COPY config/ config/
COPY hack/ hack/
COPY Makefile Makefile

RUN apk add make bash

ENV CGO_ENABLED=0

ENTRYPOINT [ "make", "test" ]

###############################################################################
FROM ghcr.io/nearnodeflash/nnf-mfu:latest as production

RUN apt update

RUN apt install -y openmpi-bin

# TODO Remove this
RUN apt install -y bash

# The following lines are from the mpiFileUtils (nnf-mfu) Dockerfile; 
# do not change them unless you know what it is you are doing
ARG port=2222
RUN sed -i "s/[ #]\(.*StrictHostKeyChecking \).*/ \1no/g" /etc/ssh/ssh_config \
    && sed -i "s/[ #]\(.*Port \).*/ \1$port/g" /etc/ssh/ssh_config \
    && echo "    UserKnownHostsFile /dev/null" >> /etc/ssh/ssh_config

# Copy the executable and execute
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]

###############################################################################
FROM production as debug
WORKDIR /workspace
COPY . .

WORKDIR /
COPY --from=builder /go/bin/dlv /dlv
COPY --from=builder /workspace/manager /manager-debug