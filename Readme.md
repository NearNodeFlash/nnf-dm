# NNF Data Movement

## Setup

1. Run the `_setup-kind.sh` script to setup a KinD cluster
2. Run `make docker-build && make kind-push` to build and upload the docker image to KinD
3. Run `_setup-cluster.sh [lustre | xfs]` to setup your cluster for lustre or xfs, accordingly
4. Run `_create-dm.sh [lustre | xfs]` to create a data movement resource for lustre or xfs, accordingly
5. Run `source _aliases.sh` at any time to load some nice shell aliases to interrogate the KinD environment

## Custom Resource Definitions

### Data Movement CRD

Describes the data movement request at the very top level. References the Servers and Computes that are part of the request.

### Rsync Template CRD

The template for the Rsync Daemon Set that describes what is deployed to Rsync Nodes. Watches LustreFileSystems to ensure the proper PV/PVCs exist for each node.

### Rsync Node Data Movement CRD

Describes a Rsync Data Movement request on an Rsync Node.

## Bootstrapping

This repository was bootstrapped using the operator-sdk

```
operator-sdk init --domain cray.hpe.com --repo github.com/NearNodeFlash/nnf-dm
operator-sdk create api --group dm --version v1alpha1 --kind DataMovement --resource --controller
operator-sdk create api --group dm --version v1alpha1 --kind NnfDataMovementManager --resource --controller

```
