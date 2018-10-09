package model

import (
	"context"
	"encoding/json"
	"net"

	"github.com/coreos/etcd/clientv3"
	"github.com/cybozu-go/coil"
)

// AssignBlock assign block to target node
func (m Model) AssignBlock(ctx context.Context, node, poolName string) (*net.IPNet, error) {
	bkeyPrefix := blockKeyPrefix(poolName)
RETRY:
	gresp, err := m.etcd.Get(ctx, bkeyPrefix, clientv3.WithPrefix())
	rev := gresp.Kvs[0].ModRevision
	if err != nil {
		return nil, err
	}
	if gresp.Count == 0 {
		return nil, ErrNotFound
	}

	var ba coil.BlockAssignment
	var bkey string
	for _, kv := range gresp.Kvs {
		var b coil.BlockAssignment
		err = json.Unmarshal(kv.Value, &b)
		if err != nil {
			return nil, err
		}
		if len(b.FreeList) != 0 {
			ba = b
			bkey = string(kv.Key)
			break
		}
	}
	if len(bkey) == 0 {
		return nil, ErrFullBlock
	}

	first := ba.FreeList[0]
	rest := ba.FreeList[1:]
	ba.Nodes[node] = append(ba.Nodes[node], first)
	ba.FreeList = rest

	output, err := json.Marshal(ba)
	if err != nil {
		return nil, err
	}
	tresp, err := m.etcd.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(bkey), "=", rev)).
		Then(clientv3.OpPut(bkey, string(output))).
		Commit()
	if err != nil {
		return nil, err
	}
	if !tresp.Succeeded {
		goto RETRY
	}
	return first, nil
}

//func (m Model) ReleaseBlock(ctx context.Context, node, pool string, subnet *net.IPNet, block *net.IPNet) error {
//	bkey := blockKey(pool, subnet)
//
//RETRY:
//	gresp, err := m.etcd.Get(ctx, bkey)
//	if err != nil {
//		return err
//	}
//	if gresp.Count == 0 {
//		return ErrNotFound
//	}
//	rev := gresp.Kvs[0].ModRevision
//	var ba coil.BlockAssignment
//	err = json.Unmarshal(gresp.Kvs[0].Value, &ba)
//	if err != nil {
//		return err
//	}
//
//	err = ba.ReleaseBlock(node, block)
//	if err == coil.ErrBlockNotFound {
//		return ErrNotFound
//	} else if err != nil {
//		return err
//	}
//
//	output, err := json.Marshal(ba)
//	if err != nil {
//		return err
//	}
//	tresp, err := m.etcd.Txn(ctx).
//		If(clientv3.Compare(clientv3.ModRevision(bkey), "=", rev)).
//		Then(clientv3.OpPut(bkey, string(output))).
//		Commit()
//	if err != nil {
//		return err
//	}
//	if !tresp.Succeeded {
//		goto RETRY
//	}
//	return nil
//}
