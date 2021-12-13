package controllers

import (
	"context"
	"fmt"
	"net"
	"sync"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/founat"
	"github.com/cybozu-go/coil/v2/pkg/ipam"
	"github.com/vishvananda/netlink"
)

type mockPoolManager struct {
	mu        sync.Mutex
	dropped   map[string]int
	synced    map[string]int
	allocated int
	used      bool
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

func (pm *mockPoolManager) AllocateBlock(ctx context.Context, poolName, nodeName, requestName string) (*coilv2.AddressBlock, error) {
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

func (pm *mockPoolManager) IsUsed(ctx context.Context, name string) (bool, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.used, nil
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

func (pm *mockPoolManager) SetUsed(used bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.used = used
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

func (n *mockNodeIPAM) NodeInternalIP(ctx context.Context) (net.IP, net.IP, error) {
	panic("not implemented")
}

type mockFoUTunnel struct {
	mu    sync.Mutex
	peers map[string]bool
}

var _ founat.FoUTunnel = &mockFoUTunnel{}

func (t *mockFoUTunnel) Init() error {
	panic("not implemented")
}

func (t *mockFoUTunnel) AddPeer(ip net.IP) (netlink.Link, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.peers[ip.String()] = true
	return nil, nil
}

func (t *mockFoUTunnel) DelPeer(ip net.IP) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.peers, ip.String())
	return nil
}

func (t *mockFoUTunnel) GetPeers() map[string]bool {
	m := make(map[string]bool)

	t.mu.Lock()
	defer t.mu.Unlock()

	for k := range t.peers {
		m[k] = true
	}
	return m
}

type mockEgress struct {
	mu  sync.Mutex
	ips map[string]bool
}

var _ founat.Egress = &mockEgress{}

func (e *mockEgress) Init() error {
	panic("not implemented")
}

func (e *mockEgress) AddClient(ip net.IP, _ netlink.Link) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ips[ip.String()] = true
	return nil
}

func (e *mockEgress) GetClients() map[string]bool {
	m := make(map[string]bool)

	e.mu.Lock()
	defer e.mu.Unlock()

	for k := range e.ips {
		m[k] = true
	}
	return m
}
