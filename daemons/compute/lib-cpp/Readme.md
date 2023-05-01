# Copy Offload API C++ Library

This library is a wrapper around the gRPC header file (`datamovement.grpc.pb.h`) that is generated
from the `../api/datamovement.proto` file.

It is recommended that you use gRPC directly (see the `client-cpp/` example) rather than this
library, but this can be used to abstract the user from the generated gRPC interface.

*Note*: This library has to be updated manually, meaning it can easily become out of date from the
*API that is defined by the `../api/datamovement.proto` file.

## Building

Run `make` to build the library. The resulting library (`libnnfdm.dylib`) is placed in the
`cmake/build/` directory

## Example client using lib-cpp

The `example/` directory contains a very simple example of a c++ client that uses the `lib-cpp`
library. Run `make` to build it and the resulting binary will be placed in the `cmake/build/`
directory.
