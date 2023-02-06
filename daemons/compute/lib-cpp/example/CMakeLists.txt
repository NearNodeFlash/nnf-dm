cmake_minimum_required(VERSION 3.5.1)

project(ExampleClient VERSION 0.0.1 LANGUAGES C CXX)

set (CMAKE_CXX_STANDARD 14)
set(CMAKE_BUILD_TYPE Debug)

add_executable(client "client.cc")
target_link_directories(client BEFORE
  PRIVATE ../cmake/build/
)
target_link_libraries(client libnnfdm.dylib)
target_include_directories(client PRIVATE ../)
