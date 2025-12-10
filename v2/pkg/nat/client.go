package nat

import (
	"net"

	"github.com/vishvananda/netlink"
)

// Client represents the interface for NAT client
// This can be re-initialized by calling `Init` again.
type Client interface {
	Init() error
	IsInitialized() (bool, error)
	SyncNat(link netlink.Link, subnets []*net.IPNet, originatingOnly bool) error
}
