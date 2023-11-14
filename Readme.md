# NNF Data Movement

## Setup

1. Run the `_setup-kind.sh` script to setup a KinD cluster
2. Run `make docker-build && make kind-push` to build and upload the docker image to KinD
3. Run `_setup-cluster.sh [lustre | xfs]` to setup your cluster for lustre or xfs, accordingly
4. Run `_create-dm.sh [lustre | xfs]` to create a data movement resource for lustre or xfs, accordingly
5. Run `source _aliases.sh` at any time to load some nice shell aliases to interrogate the KinD environment
