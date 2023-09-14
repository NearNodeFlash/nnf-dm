# Data Movement

This readme explains the Data Movement Daemon (nnf-dm) and its corresponding API (Copy Offload API).

There are also some example clients that show how to use the Copy Offload API.

## Data Movement Daemon

The nnf-dm service is a compute resident daemon that provides an interface for submitting copy offload requests to the Rabbit.

### Build

The nnf-dm service resides in [./server](./server) directory and can be built using `make build-daemon`.

### Installation

Copy the nnf-dm daemon to a suitable location such as `/usr/bin`. After defining the necessary [environmental variables](#environmental-variables), install the daemon by running `nnf-dm install`. Verify the daemon is running using `systemctl status nnf-dm`

#### Authentication

NNF software defines a Kubernetes service account for communicating data movement requests to the kubeapi server. The token file and certificate file can be downloaded by running `cert-load.sh` and verified with `cert-test.sh` The token file and certificate file must be provided to the nnf-dm daemon.

#### Customizing the Daemon Configuration

Installing the nnf-dm daemon will create a default configuration located at `/etc/systemd/system/nnf-dm.service`. The default configuration created on installation is sparse as the use of environmental variables is assumed. If desired, one can edit the configuration with the [command line options](#command-line-options). An example is show below.

```conf
[Unit]
Description=Near-Node Flash (NNF) Data Movement Service

[Service]
PIDFile=/var/run/nnf-dm.pid
ExecStartPre=/bin/rm -f /var/run/nnf-dm.pid
ExecStart=/usr/bin/nnf-dm \
   --kubernetes-service-host=172.0.0.1 \
   --kubernetes-service-port=7777 \
   --nnf-data-movement-service-token-file=/path/to/service.token \
   --nnf-data-movement-service-cert-file=/path/to/cert.file \
   --nnf-node-name=rabbit-01
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

#### Daemon Configuration Overrides

systemd can be used to create an override file that is useful for non-standard behavior of the data movement service. Run `systemctl edit nnf-dm` to create or edit the override file.

For example, running the nnf-dm service in simulation mode is achieved with the following (the empty ExecStart is required)

```conf
[Service]
ExecStart=
ExecStart=/root/nnf-dm --simulated
```

### Environmental Variables

| Name | Description |
| ---- | ----------- |
| `KUBERNETES_SERVICE_HOST` | Specifies the kubernetes service host |
| `KUBERNETES_SERVICE_PORT` | Specifies the kubernetes service port |
| `NODE_NAME` | Specifies the node name of this host |
| `NNF_NODE_NAME` | Specifies the NNF node name connected to this host |
| `NNF_DATA_MOVEMENT_SERVICE_TOKEN_FILE` | Specifies the path to the NNF Data Movement service token file |
| `NNF_DATA_MOVEMENT_SERVICE_CERT_FILE`  | Specifies the path to the NNF Data Movement service certificate file |

### Command Line Options

Command line options are defined below. Most default to their corresponding environmental variables of the same name in upper-case with hyphens replaced by underscores

| Option | Default | Description |
| ------ | ------- | ----------- |
| `--socket=[PATH]` | `/var/run/nnf-dm.sock` | Specifies the listening socket for the NNF Data Movement daemon |
| `--kubernetes-service-host=[HOST]` | `KUBERNETES_SERVICE_HOST` | Specifies the kubernetes service host |
| `--kubernetes-service-port=[PORT]` | `KUBERNETES_SERVICE_PORT` | Specifies the kubernetes service port |
| `--node-name=[NAME]` | `NODE_NAME` | Specifies the node name of this host |
| `--nnf-node-name=[NAME]` | `NNF_NODE_NAME` | Specifies the NNF node name connected to this host |
| `--nnf-data-movement-service-token-file=[PATH]` | `NNF_DATA_MOVEMENT_SERVICE_TOKEN_FILE` | Specifies the path to the NNF Data Movement service token file |
| `--nnf-data-movement-service-cert-file=[PATH]` | `NNF_DATA_MOVEMENT_SERVICE_CERT_FILE`  | Specifies the path to the NNF Data Movement service certificate file |

## Data Movement Interface (Copy Offload API)

The nnf-dm service uses Protocol Buffers to define a set of APIs for initiating, querying, canceling and deleting data movement requests. The definitions for these can be found in [./api/datamovement.proto](./api/datamovement.proto) file.

### Common fields for all requests

Each nnf-dm request contains a Workflow message containing the Name and Namespace fields. WLM must provide these values to the user application that are then provided to the data-movement API. The recommended implementation is to use environmental variables.

#### Workflow

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | Name of the workflow that is associated with this data movement request. WLM must provide this as an environmental variable (i.e. `DW_WORKFLOW_NAME`) |
| `namespace` | string | Namespace of the workflow that is associated with this data movement request. WLM can provide this as an environmental variable (i.e. `DW_WORKFLOW_NAMESPACE`) |

### API Definition

See the generated [Copy Offload API Documentation](./copy-offload-api.html) for the full API definition.

This file is generated by running `./proto-gen.sh`, which also creates example clients for Go and Python. See more below.

## Example Clients

### Example Client Generation

#### Go and Python

To build the generated code used for the Go and Python example clients, run `./proto-gen.sh`. This will also generate the documentation for the API.

`client-go` and `client-py` will have updates to the generated files when the API changes.

To run this script, you will need the following installed on your system:

- [protoc](https://grpc.io/docs/protoc-installation/)
- [protoc-gen-doc](https://github.com/pseudomuto/protoc-gen-doc#installation)
- [grpc and grpc tools python modules](https://grpc.io/docs/languages/python/quickstart/)

On OSX, this can be done easily via:

```shell
brew install protobuf protoc-gen-go protoc-gen-go-grpc
pip3 install grpcio grpcio_tools
```

#### C and C++

For the C and C++ clients, the clients must be built to generate the source code to support the API. Run the `Makefiles` in the `_client-c`, `client-cpp`, and `lib-cpp` directories to update the generated API files.

**Note**: `lib-cpp` must still be updated manually to support changes to the API, as it is a wrapper around the generated C++ files.

### Go

The Go client, which resides in `client-go/`, is the most customizable client example. It demonstrates a number of command line options to configure the Create request. It provides a debug hook to _not_ delete the request after completion, so one can test that the requests are cleaned up as part of the workflow.

### C

The C client, which resides in `_client-c/`, is a simple client that uses CGo as an interface to the server. (The leading underscore "\_" in the directory is so Go ignores directory as part of a larger repository build). As there is no native C GRPC implementation, Go is used to interface with the GRPC server while providing an interface to the C code. A Makefile is provided to build the various components and assemble it into a final executable.

The C client should not be used for anything more than rapid prototyping. No new features will be added to the C client.

### C++

The C++ client, which resides in `client-cpp/`, is a simple client that uses [gRPC C++](https://grpc.io/docs/languages/cpp/).

### Python

The Python client, which resides in `client-py/`, is a very simple client. No customization options are provided
