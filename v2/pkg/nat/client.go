package nat

import (
	"net"

	"github.com/vishvananda/netlink"
)

// Client is the interface for a NAT client that routes egress traffic
// through a tunnel link.
type Client interface {
	// Init prepares the routing rules required by SyncNat (link-local,
	// in-cluster, narrow and wide table rules). It is idempotent and may
	// be called again to re-initialize the client; doing so clears any
	// routes previously added by SyncNat.
	Init() error

	// IsInitialized reports whether Init has been completed for the
	// IP families configured on this client.
	IsInitialized() (bool, error)

	// SyncNat reconciles the egress routes on link to match subnets.
	// Subnets present in the destination tables but absent from the
	// argument are removed; subnets in the argument that are not yet
	// present are added. When originatingOnly is true, FWMark and
	// connmark rules are configured so that only traffic originating
	// from the egress namespace is NAT'd; when false, those rules are
	// removed. Init must have been called beforehand.
	SyncNat(link netlink.Link, subnets []*net.IPNet, originatingOnly bool) error
}
