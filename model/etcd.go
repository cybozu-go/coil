package model

import (
	"context"
	"encoding/json"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/clientv3util"
	"github.com/cybozu-go/coil"
)

// Model implements storage operations.
type Model struct {
	etcd *clientv3.Client
}

// AddPool adds a new address pool.
func (m Model) AddPool(ctx context.Context, name string, pool *coil.AddressPool) error {
	data, err := json.Marshal(pool)
	if err != nil {
		return err
	}

	key := poolKey(name)
	resp, err := m.etcd.Txn(ctx).
		If(clientv3util.KeyMissing(key)).
		Then(clientv3.OpPut(key, string(data))).
		Commit()
	if err != nil {
		return err
	}
	if !resp.Succeeded {
		return ErrConflicted
	}
	return nil
}
