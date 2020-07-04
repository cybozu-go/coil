package model

import (
	"bytes"
	"context"
	"encoding/json"
	"net"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/clientv3util"
	"github.com/cybozu-go/coil/v1"
	"github.com/cybozu-go/log"
)

func (m etcdModel) GetMyBlocks(ctx context.Context, node string) (map[string][]*net.IPNet, error) {
	resp, err := m.etcd.Get(ctx, keyBlock, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	ret := make(map[string][]*net.IPNet)
	for _, kv := range resp.Kvs {
		ba := new(coil.BlockAssignment)
		err = json.Unmarshal(kv.Value, ba)
		if err != nil {
			return nil, err
		}

		blocks := ba.Nodes[node]
		if len(blocks) > 0 {
			t := kv.Key[len(keyBlock):]
			poolName := string(t[0:bytes.IndexByte(t, '/')])
			ret[poolName] = append(ret[poolName], blocks...)
		}
	}
	return ret, nil
}

func (m etcdModel) GetAssignedBlocks(ctx context.Context) (map[string][]*net.IPNet, error) {
	resp, err := m.etcd.Get(ctx, keyBlock, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	ret := make(map[string][]*net.IPNet)
	for _, kv := range resp.Kvs {
		ba := new(coil.BlockAssignment)
		err = json.Unmarshal(kv.Value, ba)
		if err != nil {
			return nil, err
		}

		t := kv.Key[len(keyBlock):]
		poolName := string(t[0:bytes.IndexByte(t, '/')])
		for _, blocks := range ba.Nodes {
			ret[poolName] = append(ret[poolName], blocks...)
		}
	}
	return ret, nil
}

func (m etcdModel) AcquireBlock(ctx context.Context, node, poolName string) (*net.IPNet, error) {
	bkeyPrefix := blockKeyPrefix(poolName)
RETRY:
	gresp, err := m.etcd.Get(ctx, bkeyPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	if gresp.Count == 0 {
		return nil, ErrNotFound
	}

	var ba coil.BlockAssignment
	var bkey string
	var rev int64
	for _, kv := range gresp.Kvs {
		var b coil.BlockAssignment
		err = json.Unmarshal(kv.Value, &b)
		if err != nil {
			return nil, err
		}
		if len(b.FreeList) != 0 {
			ba = b
			bkey = string(kv.Key)
			rev = kv.ModRevision
			break
		}
	}
	if len(bkey) == 0 {
		return nil, ErrOutOfBlocks
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

func (m etcdModel) ReleaseBlock(ctx context.Context, node, poolName string, block *net.IPNet, force bool) error {
	pool, err := m.GetPool(ctx, poolName)
	if err != nil {
		return err
	}

	var subnet *net.IPNet
	for _, sn := range pool.Subnets {
		if sn.Contains(block.IP) {
			subnet = sn
			break
		}
	}
	if subnet == nil {
		return ErrNotFound
	}

	bkey := blockKey(poolName, subnet)
RETRY:
	gresp, err := m.etcd.Get(ctx, bkey)
	if err != nil {
		return err
	}
	if gresp.Count == 0 {
		log.Error("block assignment information has been lost for "+bkey, nil)
		return ErrNotFound
	}
	rev := gresp.Kvs[0].ModRevision
	var ba coil.BlockAssignment
	err = json.Unmarshal(gresp.Kvs[0].Value, &ba)
	if err != nil {
		return err
	}

	err = ba.ReleaseBlock(node, block)
	if err == coil.ErrBlockNotFound {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	output, err := json.Marshal(ba)
	if err != nil {
		return err
	}

	var thenOps []clientv3.Op
	if force {
		thenOps = []clientv3.Op{
			clientv3.OpPut(bkey, string(output)),
			clientv3.OpDelete(ipKeyPrefix(block), clientv3.WithPrefix()),
		}
	} else {
		thenOps = []clientv3.Op{
			clientv3.OpTxn(
				[]clientv3.Cmp{clientv3util.KeyMissing(ipKeyPrefix(block)).WithPrefix()},
				[]clientv3.Op{clientv3.OpPut(bkey, string(output))},
				nil,
			),
		}
	}

	tresp, err := m.etcd.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(bkey), "=", rev)).
		Then(thenOps...).Commit()
	if err != nil {
		return err
	}
	if !tresp.Succeeded {
		goto RETRY
	}
	return nil
}
