# Basic C Client

A basic example of a data movement client written in C and CGo.

**WARNING: The C client should not be used beyond rapid prototyping; The C++ client should be use long-term.**

## Generating client code
To generate the gRPC client interfaces from our .proto service definition, use the `protoc` protocol buffer compiler
```
$ make protoc
```
Running this command generate the following files in your current directory
- datamovement.pb-c.h, the header which declares your generated message classes
- datamovement.pb-c.c, which contains the implementation of your message classes

## Creating the Client

Since there is no native C gRPC Client, this example uses the gRPC Go client with a CGo interface for interacting with the client.

```
$ make client.a
```

## Compile and Run
Build the client
```
$ make client
```
Assuming you have a server running (i.e. `./server --simulated --socket=/tmp/nnf.sock`) Run the client
```
$ ./client
```
