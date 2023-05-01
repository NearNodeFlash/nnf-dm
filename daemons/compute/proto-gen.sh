#!/bin/bash

# Copyright 2021-2023 Hewlett Packard Enterprise Development LP
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

if ! command -v protoc &> /dev/null; then
    echo "protoc is not installed"
    echo "see https://grpc.io/docs/protoc-installation/"
    exit 1
fi

if ! command -v protoc-gen-doc &> /dev/null; then
    echo "protoc-doc-gen plugin is not installed"
    echo "run `go install  github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@latest`"
    exit 1
fi

protoc --go_out=paths=source_relative:./client-go --go-grpc_out=paths=source_relative:./client-go \
    --doc_out=. --doc_opt=html,copy-offload-api.html \
    ./api/datamovement.proto \

if ! command -v python3 -c "import grpc_tools.protoc" &> /dev/null; then
    echo "python grpc_tools.protoc module is not installed"
    exit 1
fi

python3 -m grpc_tools.protoc -I./api --python_out=./client-py --grpc_python_out=./client-py \
    ./api/datamovement.proto
