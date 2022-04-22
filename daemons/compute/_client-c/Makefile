# Copyright 2021, 2022 Hewlett Packard Enterprise Development LP
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

all : client.a client

client.a : client.go
	GODEBUG=cgocheck=2 go build -buildmode=c-archive client.go

client : client.a client.c client.h rsyncdatamovement.pb-c.h
	gcc -pthread client.c client.a -o client

# protoc is used to generate the typedefs; no actual C code is used since
# it is not using GRPC (it uses protoc-c-rpc, which is a low-level protocol)
protoc :
	protoc --c_out=. --proto_path=../api ../api/rsyncdatamovement.proto

clean :
	rm client client.a client.h
