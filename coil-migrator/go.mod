module github.com/cybozu-go/coil/coil-migrator

go 1.16

require (
	github.com/cybozu-go/coil v1.1.9
	github.com/cybozu-go/coil/v2 v2.0.6
	github.com/cybozu-go/etcdutil v1.3.5
	github.com/cybozu-go/netutil v1.4.1
	github.com/spf13/cobra v1.1.3
	k8s.io/api v0.20.7
	k8s.io/apimachinery v0.20.7
	k8s.io/client-go v0.20.7
	sigs.k8s.io/controller-runtime v0.8.3
)

replace (
	github.com/cybozu-go/coil => ../v1
	github.com/cybozu-go/coil/v2 => ../v2
	google.golang.org/grpc => google.golang.org/grpc v1.26.0
)
