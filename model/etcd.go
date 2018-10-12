package model

import (
	"github.com/coreos/etcd/clientv3"
)

// etcdModel implements Model on etcd.
type etcdModel struct {
	etcd *clientv3.Client
}

// NewEtcdModel returns a Model that works on etcd.
func NewEtcdModel(etcd *clientv3.Client) Model {
	return etcdModel{etcd}
}
