package founat

import (
	"fmt"
	"net"
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
)

const (
	egressTableID    = 118
	egressProtocolID = 30
	egressRulePrio   = 2000

	egressDummy = "egress-dummy"
)

// Egress represents NAT and routing service running on egress Pods.
// Methods are idempotent; i.e. they can be called multiple times.
type Egress interface {
	Init() error
	AddClient(net.IP, netlink.Link) error
}

// NewEgress creates an Egress
func NewEgress(iface string, ipv4, ipv6 net.IP) Egress {
	if ipv4 != nil && ipv4.To4() == nil {
		panic("invalid IPv4 address")
	}
	if ipv6 != nil && ipv6.To4() != nil {
		panic("invalid IPv6 address")
	}
	return &egress{
		iface: iface,
		ipv4:  ipv4,
		ipv6:  ipv6,
	}
}

type egress struct {
	iface string
	ipv4  net.IP
	ipv6  net.IP

	mu sync.Mutex
}

func (e *egress) newRule(family int) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = family
	r.IifName = e.iface
	r.Table = egressTableID
	r.Priority = egressRulePrio
	return r
}

func (e *egress) Init() error {
	// avoid double initialization in case the program restarts
	_, err := netlink.LinkByName(egressDummy)
	if err == nil {
		return nil
	}
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return err
	}

	if e.ipv4 != nil {
		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
		if err != nil {
			return err
		}
		ipn := netlink.NewIPNet(e.ipv4)
		err = ipt.Append("nat", "POSTROUTING", "!", "-s", ipn.String(), "-o", e.iface, "-j", "MASQUERADE")
		if err != nil {
			return fmt.Errorf("failed to setup masquerade rule for IPv4: %w", err)
		}

		rule := e.newRule(netlink.FAMILY_V4)
		if err := netlink.RuleAdd(rule); err != nil {
			return fmt.Errorf("netlink: failed to add egress rule for IPv4: %w", err)
		}
	}
	if e.ipv6 != nil {
		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
		if err != nil {
			return err
		}
		ipn := netlink.NewIPNet(e.ipv6)
		err = ipt.Append("nat", "POSTROUTING", "!", "-s", ipn.String(), "-o", e.iface, "-j", "MASQUERADE")
		if err != nil {
			return fmt.Errorf("failed to setup masquerade rule for IPv6: %w", err)
		}

		rule := e.newRule(netlink.FAMILY_V6)
		if err := netlink.RuleAdd(rule); err != nil {
			return fmt.Errorf("netlink: failed to add egress rule for IPv6: %w", err)
		}
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = egressDummy
	return netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs})
}

func (e *egress) AddClient(addr net.IP, link netlink.Link) error {
	// Note:
	// The following checks are not necessary in fact because,
	// prior to this point, the support for the IP family is tested
	// by FouTunnel.AddPeer().  If the test fails, then no `link`
	// is created and this method will not be called.
	// Just as a safeguard.
	if addr.To4() != nil && e.ipv4 == nil {
		return ErrIPFamilyMismatch
	}
	if addr.To4() == nil && e.ipv6 == nil {
		return ErrIPFamilyMismatch
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	family := netlink.FAMILY_V4
	if addr.To4() == nil {
		family = netlink.FAMILY_V6
	}
	routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: egressTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: failed to list routes in table %d: %w", egressTableID, err)
	}

	for _, r := range routes {
		if r.Dst == nil {
			continue
		}
		if r.Dst.IP.Equal(addr) {
			return nil
		}
	}

	// link up here to minimize the down time
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("netlink: failed to link up %s: %w", link.Attrs().Name, err)
	}
	err = netlink.RouteAdd(&netlink.Route{
		Dst:       netlink.NewIPNet(addr),
		LinkIndex: link.Attrs().Index,
		Table:     egressTableID,
		Protocol:  egressProtocolID,
	})
	if err != nil {
		return fmt.Errorf("netlink: failed to add %s to table %d: %w", addr.String(), egressTableID, err)
	}
	return nil
}
