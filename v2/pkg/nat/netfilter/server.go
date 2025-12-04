package netfilter

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"sync"

	"github.com/vishvananda/netlink"

	"github.com/cybozu-go/coil/v2/pkg/nat"
)

// Backend constants for NAT implementation.
const (
	BackendIptables = "iptables"
	BackendNftables = "nftables"
)

const (
	linkName = "egress-dummy"
)

const (
	nsTableID    = 118
	nsProtocolID = 30
	nsRulePrio   = 2000
)

const (
	natTable    = "nat"
	natChain    = "POSTROUTING"
	filterTable = "filter"
	filterChain = "FORWARD"
)

var _ nat.Server = &NatServer{}

// NatServer implements the nat.Server interface using netfilter (iptables or nftables)
// for NAT masquerading and routing rules.
type NatServer struct {
	iface   string
	ipv4    net.IP
	ipv6    net.IP
	backend string

	clients map[string]struct{}
	mu      sync.RWMutex
}

// NewNatServer creates a new NatServer that performs NAT on the specified interface.
// It sets up masquerade rules and FIB rules for the given IPv4 and/or IPv6 addresses
// using the specified backend (iptables or nftables).
func NewNatServer(iface string, ipv4, ipv6 net.IP, backend string) (*NatServer, error) {
	n := &NatServer{
		iface:   iface,
		ipv4:    ipv4,
		ipv6:    ipv6,
		backend: backend,
		clients: make(map[string]struct{}),
	}
	if err := n.init(); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *NatServer) AddClient(ip net.IP, link netlink.Link) error {
	// Note:
	// The following checks are not necessary in fact because,
	// prior to this point, the support for the IP family is tested
	// by FouTunnel.AddPeer().  If the test fails, then no `link`
	// is created and this method will not be called.
	// Just as a safeguard.
	if isIPv4(ip) && n.ipv4 == nil {
		return nat.ErrIPFamilyMismatch
	}
	if !isIPv4(ip) && n.ipv6 == nil {
		return nat.ErrIPFamilyMismatch
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	family := netlink.FAMILY_V4
	if ip.To4() == nil {
		family = netlink.FAMILY_V6
	}

	rs, err := netlink.RouteListFiltered(family, &netlink.Route{Table: nsTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: failed to list routes in table %d: %w", nsTableID, err)
	}

	for _, r := range rs {
		if r.Dst == nil {
			continue
		}
		if r.Dst.IP.Equal(ip) {
			return nil
		}
	}

	// link up here to minimize the down time
	// See https://github.com/cybozu-go/coil/issues/287.
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("netlink: failed to link up %s: %w", link.Attrs().Name, err)
	}

	err = netlink.RouteAdd(&netlink.Route{
		Dst:       netlink.NewIPNet(ip),
		LinkIndex: link.Attrs().Index,
		Table:     nsTableID,
		Protocol:  nsProtocolID,
	})
	if err != nil {
		return fmt.Errorf("netlink: failed to add %s to table %d: %w", ip.String(), nsTableID, err)
	}

	n.clients[ip.String()] = struct{}{}
	return nil
}

func (n *NatServer) GetClients() map[string]struct{} {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return maps.Clone(n.clients)
}

func (n *NatServer) init() error {
	l, err := netlink.LinkByName(linkName)
	if !errors.As(err, new(netlink.LinkNotFoundError)) {
		return err
	}
	// if the link already exists, do nothing
	if l != nil {
		return nil
	}

	if n.ipv4 != nil {
		if err := n.initIPv4(); err != nil {
			return fmt.Errorf("failed to init IPv4: %w", err)
		}
	}

	if n.ipv6 != nil {
		if err := n.initIPv6(); err != nil {
			return fmt.Errorf("failed to init IPv6: %w", err)
		}
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = linkName
	if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
		return fmt.Errorf("failed to add link: %w", err)
	}
	return nil
}

func (n *NatServer) initIPv4() error {
	switch n.backend {
	case BackendIptables:
		if err := setIPTablesMasqRules(netlink.FAMILY_V4, n.iface, n.ipv4); err != nil {
			return err
		}
	case BackendNftables:
		if err := setNFTablesMasqRules(netlink.FAMILY_V4, n.iface, n.ipv4); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid nat backend: %s", n.backend)
	}

	return n.setFibRules(netlink.FAMILY_V4)
}

func (n *NatServer) initIPv6() error {
	switch n.backend {
	case BackendIptables:
		if err := setIPTablesMasqRules(netlink.FAMILY_V6, n.iface, n.ipv6); err != nil {
			return err
		}
	case BackendNftables:
		if err := setNFTablesMasqRules(netlink.FAMILY_V6, n.iface, n.ipv6); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid nat backend: %s", n.backend)
	}

	return n.setFibRules(netlink.FAMILY_V6)
}

func (n *NatServer) setFibRules(family int) error {
	r := netlink.NewRule()
	r.Family = family
	r.IifName = n.iface
	r.Table = nsTableID
	r.Priority = nsRulePrio

	if err := netlink.RuleAdd(r); err != nil {
		return fmt.Errorf("failed to add fib rule to egress table: %w", err)
	}
	return nil
}

func isIPv4(ip net.IP) bool {
	return ip.To4() != nil
}
