package model

import (
	"context"
	"encoding/json"
	"net"
	"strconv"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/clientv3util"
	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/netutil"
)

func (m etcdModel) GetAllocatedIPs(ctx context.Context, block *net.IPNet) (map[string]net.IP, error) {
	prefix := ipKeyPrefix(block)
	resp, err := m.etcd.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	ips := make(map[string]net.IP)
	for _, kv := range resp.Kvs {
		offsetStr := string(kv.Key[len(prefix):])
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return nil, err
		}
		ip := netutil.IntToIP4(netutil.IP4ToInt(block.IP) + uint32(offset))
		var key string
		var assignment coil.IPAssignment
		err = json.Unmarshal(kv.Value, &assignment)
		if err != nil {
			// In older than version 1.0.2, the value is container-id but not json.
			key = string(kv.Value)
		} else {
			key = assignment.ContainerID
		}
		ips[key] = ip
	}
	return ips, nil
}

func (m etcdModel) AllocateIP(ctx context.Context, block *net.IPNet, assignment coil.IPAssignment) (net.IP, error) {
	resp, err := m.etcd.Get(ctx, ipKeyPrefix(block), clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		return nil, err
	}
	allocated := make(map[string]bool)
	for _, kv := range resp.Kvs {
		allocated[string(kv.Key)] = true
	}
	ones, bits := block.Mask.Size()
	blockSize := int(1 << uint(bits-ones))

	val, err := json.Marshal(assignment)
	offset := -1
	for i := 0; i < blockSize; i++ {
		k := ipKey(block, i)
		if allocated[k] {
			continue
		}
		resp, err := m.etcd.Txn(ctx).
			If(clientv3util.KeyMissing(k)).
			Then(clientv3.OpPut(k, string(val))).
			Commit()
		if err != nil {
			return nil, err
		}
		if !resp.Succeeded {
			continue
		}
		offset = i
		break
	}
	if offset == -1 {
		return nil, ErrBlockIsFull
	}
	return netutil.IntToIP4(netutil.IP4ToInt(block.IP) + uint32(offset)), nil
}

func (m etcdModel) FreeIP(ctx context.Context, block *net.IPNet, ip net.IP, modRev int64) error {
	offset := netutil.IP4ToInt(ip) - netutil.IP4ToInt(block.IP)
	key := ipKey(block, int(offset))

	resp, err := m.etcd.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", modRev)).
		Then(clientv3.OpDelete(key)).
		Commit()
	if err != nil {
		return err
	}
	if !resp.Succeeded {
		return ErrModRevDiffers
	}
	return nil
}
