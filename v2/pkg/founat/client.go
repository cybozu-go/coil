package founat

import (
	"fmt"
	"net"
	"sync"

	"github.com/vishvananda/netlink"
)

// IDs
const (
	ncProtocolID    = 30
	ncNarrowTableID = 117
	ncWideTableID   = 118

	mainTableID = 254
)

// rule priorities
const (
	ncLinkLocalPrio = 1800
	ncNarrowPrio    = 1900
	ncLocalPrioBase = 2000
	ncWidePrio      = 2100
)

// special subnets
var (
	v4PrivateList = []*net.IPNet{
		{IP: net.ParseIP("10.0.0.0").To4(), Mask: net.CIDRMask(8, 32)},
		{IP: net.ParseIP("172.16.0.0").To4(), Mask: net.CIDRMask(12, 32)},
		{IP: net.ParseIP("192.168.0.0").To4(), Mask: net.CIDRMask(16, 32)},
	}

	v6PrivateList = []*net.IPNet{
		{IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},
	}

	v4LinkLocal = &net.IPNet{IP: net.ParseIP("169.254.0.0"), Mask: net.CIDRMask(16, 32)}

	v6LinkLocal = &net.IPNet{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)}
)

// NatClient represents the interface for NAT client
type NatClient interface {
	Init() error
	AddEgress(link netlink.Link, subnets []*net.IPNet) error
}

// NewNatClient creates a NatClient.
//
// `ipv4` and `ipv6` are IPv4 and IPv6 addresses of the client pod.
// Either one of them can be nil.
//
// `podNodeNet` is, if given, are networks for Pod and Node addresses.
// If all the addresses of Pods and Nodes are within IPv4/v6 private addresses,
// `podNodeNet` can be left nil.
func NewNatClient(ipv4, ipv6 net.IP, podNodeNet []*net.IPNet) NatClient {
	if ipv4 != nil && ipv4.To4() == nil {
		panic("invalid IPv4 address")
	}
	if ipv6 != nil && ipv6.To4() != nil {
		panic("invalid IPv6 address")
	}

	var v4priv, v6priv []*net.IPNet
	if len(podNodeNet) > 0 {
		for _, n := range podNodeNet {
			if _, bits := n.Mask.Size(); bits == 32 {
				v4priv = append(v4priv, n)
			} else {
				v6priv = append(v6priv, n)
			}
		}
	} else {
		v4priv = v4PrivateList
		v6priv = v6PrivateList
	}

	return &natClient{
		ipv4:   ipv4 != nil,
		ipv6:   ipv6 != nil,
		v4priv: v4priv,
		v6priv: v6priv,
	}
}

type natClient struct {
	ipv4 bool
	ipv6 bool

	v4priv []*net.IPNet
	v6priv []*net.IPNet

	mu sync.Mutex
}

func newRuleForClient(family, table, prio int) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = family
	r.Table = table
	r.Priority = prio
	return r
}

func (c *natClient) Init() error {
	if c.ipv4 {
		linkLocalRule := newRuleForClient(netlink.FAMILY_V4, mainTableID, ncLinkLocalPrio)
		linkLocalRule.Dst = v4LinkLocal
		if err := netlink.RuleAdd(linkLocalRule); err != nil {
			return fmt.Errorf("netlink: failed to add v4 link local rule: %w", err)
		}

		narrowRule := newRuleForClient(netlink.FAMILY_V4, ncNarrowTableID, ncNarrowPrio)
		if err := netlink.RuleAdd(narrowRule); err != nil {
			return fmt.Errorf("netlink: failed to add v4 narrow rule: %w", err)
		}

		for i, n := range c.v4priv {
			r := newRuleForClient(netlink.FAMILY_V4, mainTableID, ncLocalPrioBase+i)
			r.Dst = n
			if err := netlink.RuleAdd(r); err != nil {
				return fmt.Errorf("netlink: failed to add %s to rule: %w", n.String(), err)
			}
		}

		wideRule := newRuleForClient(netlink.FAMILY_V4, ncWideTableID, ncWidePrio)
		if err := netlink.RuleAdd(wideRule); err != nil {
			return fmt.Errorf("netlink: failed to add v4 wide rule: %w", err)
		}
	}

	if c.ipv6 {
		linkLocalRule := newRuleForClient(netlink.FAMILY_V6, mainTableID, ncLinkLocalPrio)
		linkLocalRule.Dst = v6LinkLocal
		if err := netlink.RuleAdd(linkLocalRule); err != nil {
			return fmt.Errorf("netlink: failed to add v6 link local rule: %w", err)
		}

		narrowRule := newRuleForClient(netlink.FAMILY_V6, ncNarrowTableID, ncNarrowPrio)
		if err := netlink.RuleAdd(narrowRule); err != nil {
			return fmt.Errorf("netlink: failed to add v6 narrow rule: %w", err)
		}

		for i, n := range c.v6priv {
			r := newRuleForClient(netlink.FAMILY_V6, mainTableID, ncLocalPrioBase+i)
			r.Dst = n
			if err := netlink.RuleAdd(r); err != nil {
				return fmt.Errorf("netlink: failed to add %s to rule: %w", n.String(), err)
			}
		}

		wideRule := newRuleForClient(netlink.FAMILY_V6, ncWideTableID, ncWidePrio)
		if err := netlink.RuleAdd(wideRule); err != nil {
			return fmt.Errorf("netlink: failed to add v6 wide rule: %w", err)
		}
	}

	return nil
}

func (c *natClient) AddEgress(link netlink.Link, subnets []*net.IPNet) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, n := range subnets {
		if err := c.addEgress1(link, n); err != nil {
			return err
		}
	}
	return nil
}

func (c *natClient) addEgress1(link netlink.Link, n *net.IPNet) error {
	var priv []*net.IPNet
	if n.IP.To4() != nil {
		if !c.ipv4 {
			return nil
		}
		priv = c.v4priv
	} else {
		if !c.ipv6 {
			return nil
		}
		priv = c.v6priv
	}

	for _, p := range priv {
		if !p.Contains(n.IP) {
			continue
		}

		err := netlink.RouteAdd(&netlink.Route{
			Table:     ncNarrowTableID,
			Dst:       n,
			LinkIndex: link.Attrs().Index,
			Protocol:  ncProtocolID,
		})
		if err != nil {
			return fmt.Errorf("netlink: failed to add route to %s: %w", n.String(), err)
		}
		return nil
	}

	err := netlink.RouteAdd(&netlink.Route{
		Table:     ncWideTableID,
		Dst:       n,
		LinkIndex: link.Attrs().Index,
		Protocol:  ncProtocolID,
	})
	if err != nil {
		return fmt.Errorf("netlink: failed to add route to %s: %w", n.String(), err)
	}
	return nil
}
