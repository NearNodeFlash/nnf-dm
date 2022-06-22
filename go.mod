module github.com/NearNodeFlash/nnf-dm

go 1.16

// kubeflow/common requires an old version - absent this line later versions are pulled in that are incompatible
replace k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20200805222855-6aeccd4b50c6

require (
	github.com/HewlettPackard/dws v0.0.0-20220621142357-8e32b62eca41
	github.com/HewlettPackard/lustre-csi-driver v0.0.0-20220516192757-17ac28565db5
	github.com/NearNodeFlash/lustre-fs-operator v0.0.0-20220517205036-02c092067a4c
	github.com/NearNodeFlash/nnf-sos v0.0.0-20220518142843-c31693162047
	github.com/google/uuid v1.3.0
	github.com/kubeflow/common v0.4.1
	github.com/kubeflow/mpi-operator/v2 v2.0.0-20211209024655-d7fc50603a4d
	github.com/onsi/ginkgo/v2 v2.1.4
	github.com/onsi/gomega v1.19.0
	github.com/takama/daemon v1.0.0
	go.uber.org/zap v1.21.0
	golang.org/x/sys v0.0.0-20220319134239-a9b59b0215f8
	google.golang.org/grpc v1.43.0
	google.golang.org/protobuf v1.27.1
	k8s.io/api v0.23.6
	k8s.io/apimachinery v0.23.6
	k8s.io/client-go v0.23.6
	sigs.k8s.io/controller-runtime v0.11.2
)
