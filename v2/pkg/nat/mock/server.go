package mock

import (
	"maps"
	"net"
	"sync"

	"github.com/vishvananda/netlink"

	"github.com/cybozu-go/coil/v2/pkg/nat"
)

var _ nat.Server = &NatServer{}

// NatServer is a mock implementation of nat.Server for testing purposes.
type NatServer struct {
	ips map[string]struct{}

	mu sync.RWMutex
}

// NewNatServer creates a new mock NatServer.
func NewNatServer() *NatServer {
	return &NatServer{
		ips: make(map[string]struct{}),
	}
}

func (n *NatServer) AddClient(ip net.IP, _ netlink.Link) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.ips[ip.String()] = struct{}{}
	return nil
}

func (n *NatServer) GetClients() map[string]struct{} {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return maps.Clone(n.ips)
}
