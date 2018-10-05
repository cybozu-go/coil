package model

import (
	"github.com/coreos/etcd/clientv3"
)

// Model implements storage operations.
type Model struct {
	etcd *clientv3.Client
}
