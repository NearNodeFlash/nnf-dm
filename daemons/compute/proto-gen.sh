#!/bin/bash

protoc --go_out=paths=source_relative:./client-go --go-grpc_out=paths=source_relative:./client-go \
    ./api/rsyncdatamovement.proto

python3 -m grpc_tools.protoc -I./api --python_out=./client-py --grpc_python_out=./client-py \
    ./api/rsyncdatamovement.proto