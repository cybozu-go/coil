module github.com/cybozu-go/coil/v2

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/golang/protobuf v1.4.2
	github.com/google/go-cmp v0.5.1
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/pseudomuto/protoc-gen-doc v1.3.2
	github.com/willf/bitset v1.1.11
	go.uber.org/zap v1.15.0
	google.golang.org/grpc v1.27.0
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v0.0.0-20200812184716-7d8921505e1b
	google.golang.org/protobuf v1.25.0
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	k8s.io/kube-openapi v0.0.0-20200410145947-bcb3869e6f29 // indirect
	sigs.k8s.io/controller-runtime v0.6.2
	sigs.k8s.io/controller-tools v0.3.0
)
