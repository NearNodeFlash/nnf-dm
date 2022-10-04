# Copyright 2018 gRPC authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# cmake build file for C++ helloworld example.
# Assumes protobuf and gRPC have been installed using cmake.
# See cmake_externalproject/CMakeLists.txt for all-in-one cmake build
# that automatically builds all the dependencies before building helloworld.
cmake_minimum_required(VERSION 3.11)

project(nnfdm-client-library VERSION 0.0.1 LANGUAGES C CXX)
set(CMAKE_CXX_STANDARD 14)
set(CMAKE_CXX_STANDARD_REQUIRED True)

#
# Add option for building static or shared library.  The
# default will be shared library, but cmake can generate
# static libraries by runnin "cmake -DBUILD_SHARED_LIBS=Off ..."
#
option(BUILD_SHARED_LIBS "Build using shared libraries" On)

include(common.cmake)

# Proto file
get_filename_component(hw_proto "../api/datamovement.proto" ABSOLUTE)
get_filename_component(hw_proto_path "${hw_proto}" PATH)

# Generated sources
set(hw_proto_srcs "${CMAKE_CURRENT_BINARY_DIR}/datamovement.pb.cc")
set(hw_proto_hdrs "${CMAKE_CURRENT_BINARY_DIR}/datamovement.pb.h")
set(hw_grpc_srcs "${CMAKE_CURRENT_BINARY_DIR}/datamovement.grpc.pb.cc")
set(hw_grpc_hdrs "${CMAKE_CURRENT_BINARY_DIR}/datamovement.grpc.pb.h")
set(generated_srcs ${hw_grpc_hdrs} ${hw_grpc_srcs} ${hw_proto_hdrs} ${hw_proto_srcs})
add_custom_command(
      OUTPUT "${hw_proto_srcs}" "${hw_proto_hdrs}" "${hw_grpc_srcs}" "${hw_grpc_hdrs}"
      COMMAND ${_PROTOBUF_PROTOC}
      ARGS --grpc_out "${CMAKE_CURRENT_BINARY_DIR}"
        --cpp_out "${CMAKE_CURRENT_BINARY_DIR}"
        -I "${hw_proto_path}"
        --plugin=protoc-gen-grpc="${_GRPC_CPP_PLUGIN_EXECUTABLE}"
        "${hw_proto}"
      DEPENDS "${hw_proto}")

#
# nnfdm library: libnnfdm.so for shared library and libnnfdm.a for static
#
if(BUILD_SHARED_LIBS)
  add_library(nnfdm SHARED ${generated_srcs} client.cc client.h)
else()
  add_library(nnfdm STATIC ${generated_srcs} client.cc client.h)
endif()

#
# Compile with position independant code if we are set to run as a shared library
#
set_target_properties(nnfdm PROPERTIES POSITION_INDEPENDANT_CODE ${BUILD_SHARED_LIBS})

# Include generated *.pb.h files privately as these headers are for the library's
# internal implementation.
#
target_include_directories(nnfdm PRIVATE "${CMAKE_CURRENT_BINARY_DIR}")

# Noted: _REFLECTION, _GRPC_GRPCPP, and _PROTOBUF_LIBPROTOBUF are
#        set by common.cmake
#
target_link_libraries(nnfdm
    PUBLIC
    ${_REFLECTION}
    ${_GRPC_GRPCPP}
    ${_PROTOBUF_LIBPROTOBUF}
)

#
# Export a cmake configuration file for our library so downstream users
# may (more easily) configure this library in to their cmake-based builds.
#
include(CMakePackageConfigHelpers)
configure_package_config_file(
  "${PROJECT_SOURCE_DIR}/nnfdm-config.cmake.in"
  "${PROJECT_BINARY_DIR}/nnfdm-config.cmake"
  INSTALL_DESTINATION share/cmake/nnfdm)

# Generate install of when running 'make install':
#
# [build] -- Install configuration: "Release"
# [build] -- Installing: <installdir>/include/nnfdm/client.h
# [build] -- Installing: <installdir>/lib/libnnfdm.a
# [build] -- Installing: <installdir>/share/cmake/nnfdm/nnfdm-targets.cmake
# [build] -- Installing: <installdir>/share/cmake/nnfdm/nnfdm-targets-release.cmake
# [build] -- Installing: <installdir>/share/cmake/nnfdm/nnfdm-config.cmake
#
install(FILES client.h DESTINATION include/nnfdm)
install(TARGETS nnfdm EXPORT nnfdm-targets DESTINATION lib)
install(EXPORT nnfdm-targets FILE nnfdm-targets.cmake DESTINATION share/cmake/nnfdm)  
install(FILES ${PROJECT_BINARY_DIR}/nnfdm-config.cmake DESTINATION share/cmake/nnfdm)
