module github.com/NearNodeFlash/nnf-dm

go 1.16

replace github.com/NearNodeFlash/nnf-sos => ../nnf-sos

require (
	github.com/HewlettPackard/dws v0.0.0-20220811184026-49708e243332
	github.com/NearNodeFlash/lustre-fs-operator v0.0.0-20220727174249-9b7004c2cb38
	github.com/NearNodeFlash/nnf-sos v0.0.0-20220801194142-f58084033fd8
	github.com/google/uuid v1.3.0
	github.com/onsi/ginkgo/v2 v2.1.4
	github.com/onsi/gomega v1.20.0
	github.com/takama/daemon v1.0.0
	go.uber.org/zap v1.22.0
	golang.org/x/crypto v0.0.0-20220817201139-bc19a97f63c8
	golang.org/x/sys v0.0.0-20220818161305-2296e01440c6
	google.golang.org/grpc v1.48.0
	google.golang.org/protobuf v1.28.1
	k8s.io/api v0.24.4
	k8s.io/apimachinery v0.24.4
	k8s.io/client-go v0.24.4
	k8s.io/component-base v0.24.4 // indirect
	sigs.k8s.io/controller-runtime v0.12.3
)
