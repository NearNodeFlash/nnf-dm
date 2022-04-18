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

ENV GOPRIVATE=github.hpe.com

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
FROM arti.dev.cray.com/baseos-docker-master-local/alpine:latest

RUN apk add rsync

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
