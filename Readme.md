
# Custom Resource Definitions

## Data Movement CRD

Describes the data movement request at the very top level. References the Servers and Computes that are part of the request.

## Rsync Template CRD

The template for the Rsync Daemon Set that describes what is deployed to Rsync Nodes. Watches LustreFileSystems to ensure the proper PV/PVCs exist for each node.

## Rsync Node Data Movement CRD

Describes a Rsync Data Movement request on an Rsync Node.

# Bootstrapping

This repository was bootstrapped using the operator-sdk
```
operator-sdk init --domain cray.hpe.com --repo github.hpe.com/hpe/hpc-rabsw-nnf-dm
operator-sdk create api --group dm --version v1alpha1 --kind DataMovement --resource --controller
operator-sdk create api --group dm --version v1alpha1 --kind RsyncTemplate --resource --controller
operator-sdk create api --group dm --version v1alpha1 --kind RsyncNodeDataMovement --resource --controller
```
