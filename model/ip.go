package model

import (
	"context"
	"github.com/coreos/etcd/clientv3"
	"github.com/cybozu-go/netutil"
	"net"
)

// AllocateIP allocates new IP address for container from AddressBlock
// Multiple goroutines cannot use this concurrently.
func (m Model) AllocateIP(ctx context.Context, block *net.IPNet, containerID string) (net.IP, error) {
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

	offset := -1
	for i := 0; i < blockSize; i++ {
		key := ipKey(block, i)
		if allocated[key] {
			continue
		}
		offset = i
		_, err = m.etcd.Put(ctx, key, containerID)
		if err != nil {
			return nil, err
		}
		break
	}
	if offset == -1 {
		return nil, ErrBlockIsFull
	}
	return netutil.IntToIP4(netutil.IP4ToInt(block.IP) + uint32(offset)), nil
}

// FreeIP deletes allocated IP
func (m Model) FreeIP(ctx context.Context, block *net.IPNet, ip net.IP) error {
	offset := netutil.IP4ToInt(ip) - netutil.IP4ToInt(block.IP)
	_, err := m.etcd.Delete(ctx, ipKey(block, int(offset)))
	return err
}
