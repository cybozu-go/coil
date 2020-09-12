package controllers

import (
	"context"
	"fmt"
	"net"
	"sync"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/ipam"
)

type mockPoolManager struct {
	mu        sync.Mutex
	dropped   map[string]int
	synced    map[string]int
	allocated int
}

var _ ipam.PoolManager = &mockPoolManager{}

func (pm *mockPoolManager) DropPool(name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.dropped == nil {
		pm.dropped = make(map[string]int)
	}
	pm.dropped[name]++
}

func (pm *mockPoolManager) SyncPool(ctx context.Context, name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.synced == nil {
		pm.synced = make(map[string]int)
	}
	pm.synced[name]++
	return nil
}

func (pm *mockPoolManager) AllocateBlock(ctx context.Context, poolName, nodeName string) (*coilv2.AddressBlock, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.allocated >= 2 {
		return nil, ipam.ErrNoBlock
	}

	block := &coilv2.AddressBlock{}
	block.Name = fmt.Sprintf("%s-%d", poolName, pm.allocated)
	block.Index = int32(pm.allocated)
	pm.allocated++
	return block, nil
}

func (pm *mockPoolManager) GetDropped() map[string]int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	dropped := make(map[string]int)
	for k, v := range pm.dropped {
		dropped[k] = v
	}
	return dropped
}

func (pm *mockPoolManager) GetSynced() map[string]int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	synced := make(map[string]int)
	for k, v := range pm.synced {
		synced[k] = v
	}
	return synced
}

func (pm *mockPoolManager) GetAllocated() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.allocated
}

type mockNodeIPAM struct {
	mu       sync.Mutex
	notified int
}

var _ ipam.NodeIPAM = &mockNodeIPAM{}

func (n *mockNodeIPAM) Register(ctx context.Context, poolName, containerID, iface string, ipv4, ipv6 net.IP) error {
	panic("not implemented")
}

func (n *mockNodeIPAM) GC(ctx context.Context) error {
	panic("not implemented")

}

func (n *mockNodeIPAM) Allocate(ctx context.Context, poolName, containerID, iface string) (ipv4, ipv6 net.IP, err error) {
	panic("not implemented")
}

func (n *mockNodeIPAM) Free(ctx context.Context, containerID, iface string) error {
	panic("not implemented")
}

func (n *mockNodeIPAM) Notify(req *coilv2.BlockRequest) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.notified++
}

func (n *mockNodeIPAM) GetNotified() int {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.notified
}
