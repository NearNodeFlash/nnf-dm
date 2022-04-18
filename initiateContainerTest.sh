#!/bin/bash

# This script should be run from inside a docker container; see Dockerfile testing
KUBEBUILDER_ASSETS=/usr/local/kubebuilder/bin CGO_ENABLED=0 go test ./... -coverprofile cover.out | tee results.txt
cat results.txt

grep FAIL results.txt && echo "Unit tests failure" && exit 1

echo "Unit tests successful" && rm results.txt
