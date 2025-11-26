package nat

import (
	"net"

	"github.com/vishvananda/netlink"
)

// Server defines the interface for NAT server implementations.
// It manages client registrations and routing for egress NAT functionality.
type Server interface {
	// AddClient registers a client IP address with the NAT server and sets up
	// the necessary routing rules via the specified network link.
	AddClient(net.IP, netlink.Link) error

	// GetClients returns a copy of the currently registered client IP addresses.
	GetClients() map[string]struct{}
}
