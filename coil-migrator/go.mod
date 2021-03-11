module github.com/cybozu-go/coil/coil-migrator

go 1.13

require (
	github.com/cybozu-go/coil v1.1.9
	github.com/cybozu-go/coil/v2 v2.0.5
	github.com/cybozu-go/etcdutil v1.3.4
	github.com/cybozu-go/netutil v1.4.1
	github.com/spf13/cobra v1.1.1
	k8s.io/api v0.18.14
	k8s.io/apimachinery v0.18.14
	k8s.io/client-go v0.18.14
	sigs.k8s.io/controller-runtime v0.6.3
)

replace (
	github.com/cybozu-go/coil => ../v1
	github.com/cybozu-go/coil/v2 => ../v2
	google.golang.org/grpc => google.golang.org/grpc v1.26.0
)
