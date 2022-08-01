# Data Movement
The nnf-dm service is a compute resident daemon that provides an interface for submitting copy offload requests to the Rabbit.

## Build
The nnf-dm service resides in [./server](./server) directory and can be built using `go build -o nnf-dm`

## Installation
Copy the nnf-dm daemon to a suitable location such as `/usr/bin`. After defining the necessary [environmental variables](#environmental-variables), install the daemon by running `nnf-dm install`. Verify the daemon is running using `systemctl status nnf-dm`

### Authentication
NNF software defines a Kubernetes service account for communicating data movement requests to the kubeapi server. The token file and certificate file can be download by running `cert-load.sh` and verified with `cert-test.sh` The token file and certificate file must be provided to the nnf-dm daemon.

### Customizing the Configuration
Installing the nnf-dm daemon will create a default configuration located at `/etc/systemd/system/nnf-dm.service`. The default configuration created on install is sparse as the use of environmental variables is assumed; If desired, one can edit the configuration with the [command line options](#command-line-options). An example is show below.

```
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

### Configuration Overrides
systemd can be used to create an override file that is useful for non-standard behavior of the data movement service. Run `systemctl edit nnf-dm` to create or edit the override file.

For example, running the nnf-dm service in simulation mode is achieved with the following (the empty ExecStart is required)
```
[Service]
ExecStart=
ExecStart=/root/nnf-dm --simulated
```

## Environmental Variables
| Name | Description |
| ---- | ----------- |
| `KUBERNETES_SERVICE_HOST` | Specifies the kubernetes service host |
| `KUBERNETES_SERVICE_PORT` | Specifies the kubernetes service port |
| `NODE_NAME` | Specifies the node name of this host |
| `NNF_NODE_NAME` | Specifies the NNF node name connected to this host |
| `NNF_DATA_MOVEMENT_SERVICE_TOKEN_FILE` | Specifies the path to the NNF Data Movement service token file |
| `NNF_DATA_MOVEMENT_SERVICE_CERT_FILE`  | Specifies the path to the NNF Data Movement service certificate file |


## Command Line Options
Command line options are defined below; Most default to their corresponding environmental variables of the same name in upper-case with hyphens replaced by underscores

| Option | Default | Description |
| ------ | ------- | ----------- |
| `--socket=[PATH]` | `/var/run/nnf-dm.sock` | Specifies the listening socket for the NNF Data Movement daemon |
| `--kubernetes-service-host=[HOST]` | `KUBERNETES_SERVICE_HOST` | Specifies the kubernetes service host |
| `--kubernetes-service-port=[PORT]` | `KUBERNETES_SERVICE_PORT` | Specifies the kubernetes service port |
| `--node-name=[NAME]` | `NODE_NAME` | Specifies the node name of this host |
| `--nnf-node-name=[NAME]` | `NNF_NODE_NAME` | Specifies the NNF node name connected to this host |
| `--nnf-data-movement-service-token-file=[PATH]` | `NNF_DATA_MOVEMENT_SERVICE_TOKEN_FILE` | Specifies the path to the NNF Data Movement service token file |
| `--nnf-data-movement-service-cert-file=[PATH]` | `NNF_DATA_MOVEMENT_SERVICE_CERT_FILE`  | Specifies the path to the NNF Data Movement service certificate file |


# Data Movement Interface
The nnf-dm service uses Protocol Buffers to define a set of APIs for initiating, querying, and deleting data movement requests. The definitions for these can be found in [./api/datamovement.proto](./api/datamovement.proto) file. Consulting this file should be done in addition to the context in this section to ensure the latest API definitions are used.

## Creating a data movement request

### Create Request
| Field | Type | Description |
| ----- | ---- | ----------- |
| `initiator` | string | Name of the initiating resource. Defaults to the environmental variable `NODE_NAME` |
| `target` | string | Name of the target resource which should execute this request. Defaults to the environmental variable `NNF_NODE_NAME` |
| `workflow` | string | Name of the workflow that is associated with this data movement request. WLM must provide this as an environmental variable (i.e. `DW_WORKFLOW_NAME`) |
| `namespace` | string | Namespace of the workflow that is associated with this data movement request. WLM must provide this as an environmental variable (i.e. `DW_WORKFLOW_NAMESPACE`) |
| `source` | string | The source file or directory |
| `destination` | string | The destination file or directory |
| `dryrun` | bool | If True, the rsync copy operation should evaluate the inputs but not perform the copy |

### Create Response
| Field | Type | Description |
| ----- | ---- | ----------- |
| `uid` | string | The unique identifier for the created data movement resource |

## Query the status of a data movement request

### Status Request
| Field | Type | Description |
| ----- | ---- | ----------- |
| `uid` | string | The unique identifier for the created data movement resource |
| `maxWaitTime` | int64 | The maximum time in seconds to wait for completion of the data movement resource. Negative values imply an indefinite wait |

### Status Responsee
| Field | Type | Description |
| ----- | ---- | ----------- |
| `state` | State | Current state of the data movement request |
| `status` | Status | Current status of the data movement request |
| `message` | string | String providing detailed message pertaining to the current state/status of the request |

#### Status Response `State`
| Value | Name | Description |
| ----- | ---- | ----------- |
| 0 | Pending | The request is created but has a pending status |
| 1 | Starting | The request was created and is in the act of starting |
| 2 | Running | The data movement request is actively running |
| 3 | Completed | The data movement request has completed |

#### Status Response `Status`
| Value | Name | Description |
| ----- | ---- | ----------- |
| 0 | Invalid | The request was found to be invalid. See `message` for details |
| 1 | Not Found | The request with the supplied UID was not found |
| 2 | Succeess | The request completed with success |
| 3 | Failed | The request failed. See `message` for details |


## List the data movement requests

`namespace` and `workflow` must be supplied in order to retrieve data movement
requests. An empty list of `uids` will be returned if there are no data movement
requests that match both `namespace` and `workflow`.

### List Request

| Field | Type | Description |
| ----- | ---- | ----------- |
| `workflow` | string | Name of the workflow that is associated with the data movement requests to be retrieved. |
| `namespace` | string | Namespace of the workflow that is associated with the data movement requests to be retrieved. |

### List Response

| Field | Type | Description |
| ----- | ---- | ----------- |
| `uids` | string[] | List of data movement requests associated with the given workflow and namespace. |

## Delete a data movement request

### Delete Request
| Field | Type | Description |
| ----- | ---- | ----------- |
| `uid` | string | The unique identifier for the data movement resource |

### Delete Response 
| Field | Type | Description |
| ----- | ---- | ----------- |
| `status` | Status | Deletion status of the data movement request |

#### Delete Response `Status`
| Value | Name | Description |
| ----- | ---- | ----------- |
| 0 | Invalid | The delete request was found to be invalid |
| 1 | Not Found | The request with the supplied UID was not found |
| 2 | Deleted | The data movement request was deleted successfully |
| 3 | Active | The data movement request is currently active and cannot be deleted |

# Example Clients

## Go

The Go client, which resides in ./client-go, is the most customizable client example. It demonstrates a number of command line options to configure the Create request. It provides a debug hook to _not_ delete the request after completion so one can test the requests are cleaned up as part of the workflow.

## C

The C client, which resides in ./_client-c, is a simple client that uses CGo as an interface to the server. (The leading underscore "\_" in the directory is so Go ignores directory as part of a larger repository build). As there is no native C GRPC implementation, Go is used to interface with the GRPC server while providing an interface to the C code. A Makefile is provided to build the various components and assemble it into a final executable.

## Python

The Python client, which resides in ./client-py, is a very simple client. No customization options are provided


