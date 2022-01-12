module github.hpe.com/hpe/hpc-rabsw-nnf-dm

go 1.16

require (
	github.com/google/uuid v1.3.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator v0.0.0-20220111162522-1d08349ccfda
	github.hpe.com/hpe/hpc-rabsw-nnf-sos v0.0.0-20211223141145-a7cad4f4e7e8 // indirect
	golang.org/x/sys v0.0.0-20210910150752-751e447fb3d0
	google.golang.org/grpc v1.40.0
	google.golang.org/protobuf v1.27.1
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	sigs.k8s.io/controller-runtime v0.10.2
)
