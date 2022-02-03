module github.hpe.com/hpe/hpc-rabsw-nnf-dm

go 1.16

// kubeflow/common requires an old version - absent this line later versions are pull in that are incompatible
replace k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20200805222855-6aeccd4b50c6

require (
	github.com/google/uuid v1.3.0
	github.com/kubeflow/common v0.4.1
	github.com/kubeflow/mpi-operator/v2 v2.0.0-20211209024655-d7fc50603a4d
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	github.hpe.com/hpe/hpc-rabsw-lustre-csi-driver v0.0.0-20211216142843-0b84f0d84e3f
	github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator v0.0.0-20220111162522-1d08349ccfda
	github.hpe.com/hpe/hpc-rabsw-nnf-sos v0.0.0-20220203191719-8eb782ced38a
	go.uber.org/zap v1.19.1
	golang.org/x/sys v0.0.0-20220111092808-5a964db01320
	google.golang.org/grpc v1.43.0
	google.golang.org/protobuf v1.27.1
	k8s.io/api v0.23.2
	k8s.io/apimachinery v0.23.2
	k8s.io/client-go v0.23.2
	sigs.k8s.io/controller-runtime v0.11.0
)
