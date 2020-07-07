package coil

import "github.com/cybozu-go/etcdutil"

const (
	defaultEtcdPrefix = "/coil/"
)

// NewEtcdConfig creates a new etcd config
func NewEtcdConfig() *etcdutil.Config {
	return etcdutil.NewConfig(defaultEtcdPrefix)
}
