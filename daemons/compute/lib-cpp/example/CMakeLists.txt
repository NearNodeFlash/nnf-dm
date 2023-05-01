cmake_minimum_required(VERSION 3.5.1)

project(ExampleClient VERSION 0.0.1 LANGUAGES C CXX)

set (CMAKE_CXX_STANDARD 14)
set(CMAKE_BUILD_TYPE Debug)

add_executable(client-cpp-example "client-example.cc")
target_link_directories(client-cpp-example BEFORE
  PRIVATE ../cmake/build/
)
target_link_libraries(client-cpp-example libnnfdm.dylib)
target_include_directories(client-cpp-example PRIVATE ../)
