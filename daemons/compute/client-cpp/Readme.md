# Basic C++ Client

A basic example of a data movement client written in C++

The Makefile, CMakeLists.txt, and common.cmake files are modifications to the [helloworld cpp example](https://github.com/grpc/grpc/tree/v1.46.3/examples/cpp/helloworld).

## Example code and setup

1. Follow the Quick start instructions to [build and locally install gRPC from source](https://grpc.io/docs/languages/cpp/quickstart/#install-grpc)

### Generating client code and interfaces

Run make to generate the cmake build directory and build this application:

```shell
make
```

Running this command will generate the following files in the `./cmake/build/` directory:

- `datamovement.pb.h`, the header which declares your generated message classes
- `datamovement.pb.cc`, which contains the implementation of your message classes
- `datamovement.grpc.pb.h`, the header which declares your generated service classes
- `datamovement.grpc.pb.cc`, which contains the implementation of your service classes

Collectively, these contain:

- All the protocol buffer code to populate, serialize, and retrieve requests and response message types.
- A class called `DataMover` that contains
  - A remote interface type (or _stub_) for clients to call with the methods defined in the `DataMover` service
  - Two abstract interfaces for servers to implement, also with the methods defined in the `DataMover` service

## Creating the client

### Creating a stub

To call service methods, we first need to create a stub.

First we need to create a gRPC _channel_ for our stub, specifying the server address we want to connect to - in our case using a local socket with no SSL.

```c++
grpc::CreateChannel("unix:///tmp/nnf.sock", grpc::InsecureChannelCredentials()));
```

Now we can use the channel to create our stub using the `NewStub` method provided in the `DataMover` class we generated from our .proto.

```c++
class DataMoverClient {
    public:
        DataMoverClient(std::shared_ptr<Channel> channel) : stub_(DataMover::NewStub(channel)) {}

        std::unique_ptr<DataMover::Stub> stub_;
};
```

### Calling service methods

In this example we call the blocking/synchronous versions of each method: this means that the RPC calls wait for the server to respond.

We call the following service methods:

- Create Request / Response - Create an offload request; system implementors must ensure all parameters are provided to the create request.
- Status Request / Response - Request status of an offload request.
- Delete Request / Response - Request deletion of a completed offload request.

## Compile and Run

Build the client

```shell
make
```

Assuming you have a server running (i.e. `./server --simulated --socket=/tmp/nnf.sock`), run the client:

```shell
./client
```
