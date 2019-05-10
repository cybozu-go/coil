package model

import (
	"testing"

	"github.com/coreos/etcd/clientv3"
	"github.com/cybozu-go/coil/test"
)

// etcdModel implements Model on etcd.
type etcdModel struct {
	etcd *clientv3.Client
}

// NewEtcdModel returns a Model that works on etcd.
func NewEtcdModel(etcd *clientv3.Client) Model {
	return etcdModel{etcd}
}

// NewTestEtcdModel return a Model that works on etcd for testing.
func NewTestEtcdModel(t *testing.T, clientPort string) etcdModel {
	return etcdModel{test.NewTestEtcdClient(t, clientPort)}
}
