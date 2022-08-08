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

HOST_SYTEM = $(shell uname | cut -f 1 -d_)
SYSTEM ?= $(HOST_SYSTEM)

CXX = g++
CPPFLAGS += `pkg-config --cflags protobuf grpc`
CXXFLAGS += -std=c++11

ifeq ($(SYSTEM),Darwin)
LDFLAGS += -L/usr/local/lib `pkg-config --libs protobuf grpc++` \
			-pthread \
			-pgrpc++_reflection \
			-ldl
else
LDFLAGS += -L/usr/local/lib `pkg-config --libs protobuf grpc++`\
           -pthread\
           -Wl,--no-as-needed -lgrpc++_reflection -Wl,--as-needed\
           -ldl
endif
PROTOC = protoc
GRPC_CPP_PLUGIN = grpc_cpp_plugin
GRPC_CPP_PLUGINPATH ?= `which $(GRPC_CPP_PLUGIN)`
PROTOS_PATH = ../api

vpath %.proto $(PROTOS_PATH)

client : datamovement.pb.o datamovement.grpc.pb.o client.o
	$(CXX) $^ $(LDFLAGS) -o $@

.PRECIOUS : %.grpc.pb.cc
%.grpc.pb.cc : %.proto
	$(PROTOC) --proto_path=$(PROTOS_PATH) --grpc_out=. --plugin=protoc-gen-grpc=$(GRPC_CPP_PLUGIN_PATH) $<

.PRECIOUS : %.pb.cc
%.pb.cc : %.proto
	$(PROTOC) --proto_path=$(PROTOS_PATH) --cpp_out=. $<

protoc :
	protoc --grpc_out=. --proto_path=../api --plugin=protoc-gen-grpc=`which grpc_cpp_plugin` ../api/datamovement.proto
	protoc --cpp_out=. --proto_path=../api ../api/datamovement.proto

