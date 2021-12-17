module github.hpe.com/hpe/hpc-rabsw-nnf-dm

go 1.16

replace github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator => ./.lustre-fs-operator

require (
	github.com/google/uuid v1.1.2
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator v0.0.0-20211216155555-d22b6b0ccef4
	golang.org/x/sys v0.0.0-20210817190340-bfb29a6856f2
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	sigs.k8s.io/controller-runtime v0.10.0
)
