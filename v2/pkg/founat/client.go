package founat

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
)

// IDs
const (
	ncProtocolID    = 30
	ncNarrowTableID = 117
	ncWideTableID   = 118

	mainTableID      = 254
	nonEgressTableID = 1000
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
// This can be re-initialized by calling `Init` again.
type NatClient interface {
	Init() error
	IsInitialized() (bool, error)
	AddEgress(link netlink.Link, subnets []*net.IPNet, originatingOnly bool) error
}

// NewNatClient creates a NatClient.
//
// `ipv4` and `ipv6` are IPv4 and IPv6 addresses of the client pod.
// Either one of them can be nil.
//
// `podNodeNet` is, if given, are networks for Pod and Node addresses.
// If all the addresses of Pods and Nodes are within IPv4/v6 private addresses,
// `podNodeNet` can be left nil.
func NewNatClient(ipv4, ipv6 net.IP, podNodeNet []*net.IPNet, logFunc func(string)) NatClient {
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
		ipv4:    ipv4 != nil,
		ipv6:    ipv6 != nil,
		v4priv:  v4priv,
		v6priv:  v6priv,
		logFunc: logFunc,
	}
}

type natClient struct {
	ipv4 bool
	ipv6 bool

	v4priv []*net.IPNet
	v6priv []*net.IPNet

	logFunc func(string)

	mu sync.Mutex
}

func newRuleForClient(family, table, prio int) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = family
	r.Table = table
	r.Priority = prio
	return r
}

func (c *natClient) clear(family int) error {
	var defaultGW *net.IPNet
	if family == netlink.FAMILY_V4 {
		defaultGW = &net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)}
	} else {
		defaultGW = &net.IPNet{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)}
	}

	rules, err := netlink.RuleList(family)
	if err != nil {
		return fmt.Errorf("netlink: rule list failed: %w", err)
	}
	for _, r := range rules {
		if r.Priority < 1800 || r.Priority > 2100 {
			continue
		}
		if r.Dst == nil {
			// workaround for a library issue
			r.Dst = defaultGW
		}
		if err := netlink.RuleDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a rule: %+v, %w", r, err)
		}
	}

	routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: ncNarrowTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: route list failed: %w", err)
	}
	for _, r := range routes {
		if r.Dst == nil {
			// workaround for a library issue
			r.Dst = defaultGW
		}
		if err := netlink.RouteDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a route in table %d: %+v, %w", ncNarrowTableID, r, err)
		}
	}

	routes, err = netlink.RouteListFiltered(family, &netlink.Route{Table: ncWideTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: route list failed: %w", err)
	}
	for _, r := range routes {
		if r.Dst == nil {
			// workaround for a library issue
			r.Dst = defaultGW
		}
		if err := netlink.RouteDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a route in table %d: %+v, %w", ncWideTableID, r, err)
		}
	}

	return nil
}

func (c *natClient) Init() error {
	if c.ipv4 {
		if err := c.clear(netlink.FAMILY_V4); err != nil {
			return err
		}
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
		if err := c.clear(netlink.FAMILY_V6); err != nil {
			return err
		}
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

func (c *natClient) IsInitialized() (bool, error) {
	if c.ipv4 {
		rules, err := netlink.RuleListFiltered(netlink.FAMILY_V4, &netlink.Rule{Table: mainTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return false, fmt.Errorf("netlink: failed to list v4 main rules: %w", err)
		}
		// Expect link local and private rules
		if len(rules) < (len(c.v4priv) + 1) {
			return false, nil
		}

		rules, err = netlink.RuleListFiltered(netlink.FAMILY_V4, &netlink.Rule{Table: ncNarrowTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return false, fmt.Errorf("netlink: failed to list v4 narrow rules: %w", err)
		}
		if len(rules) != 1 {
			return false, nil
		}

		rules, err = netlink.RuleListFiltered(netlink.FAMILY_V4, &netlink.Rule{Table: ncWideTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return false, fmt.Errorf("netlink: failed to list v4 wide rules: %w", err)
		}
		if len(rules) != 1 {
			return false, nil
		}
	}

	if c.ipv6 {
		rules, err := netlink.RuleListFiltered(netlink.FAMILY_V6, &netlink.Rule{Table: mainTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return false, fmt.Errorf("netlink: failed to list v6 main rules: %w", err)
		}
		// Expect link local and private rules
		if len(rules) < (len(c.v6priv) + 1) {
			return false, nil
		}

		rules, err = netlink.RuleListFiltered(netlink.FAMILY_V6, &netlink.Rule{Table: ncNarrowTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return false, fmt.Errorf("netlink: failed to list v6 narrow rules: %w", err)
		}
		if len(rules) != 1 {
			return false, nil
		}

		rules, err = netlink.RuleListFiltered(netlink.FAMILY_V6, &netlink.Rule{Table: ncWideTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return false, fmt.Errorf("netlink: failed to list v6 wide rules: %w", err)
		}
		if len(rules) != 1 {
			return false, nil
		}
	}

	return true, nil
}

func (c *natClient) AddEgress(link netlink.Link, subnets []*net.IPNet, originatingOnly bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	currentSubnets, err := collectRoutes(link.Attrs().Index)
	if err != nil {
		return err
	}

	var adds []*net.IPNet
	var deletes []netlink.Route
	for _, subnet := range subnets {
		if _, ok := currentSubnets[subnet.String()]; !ok {
			adds = append(adds, subnet)
		}
	}

	subnetsSet := subnetsSet(subnets)
	for currenSubnet, route := range currentSubnets {
		if _, ok := subnetsSet[currenSubnet]; !ok {
			deletes = append(deletes, route)
		}
	}

	for _, n := range adds {
		if err := c.addEgress1(link, n); err != nil {
			return err
		}

		if originatingOnly {
			family := iptables.ProtocolIPv6
			if n.IP.To4() != nil {
				family = iptables.ProtocolIPv4
			}
			if err := configureRoutes(family); err != nil {
				return err
			}
		}
	}

	for _, r := range deletes {
		if c.logFunc != nil {
			c.logFunc(fmt.Sprintf("removing a destination %s", r.Dst.String()))
		}
		if err := netlink.RouteDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a route  %+v, %w", r, err)
		}
	}

	return nil
}

func collectRoutes(linkIndex int) (map[string]netlink.Route, error) {
	subnets := make(map[string]netlink.Route)

	err := collectRoute1(netlink.FAMILY_V4, ncNarrowTableID, linkIndex, subnets)
	if err != nil {
		return nil, fmt.Errorf("failed to collect route %d %d %d: %w", netlink.FAMILY_V4, ncNarrowTableID, linkIndex, err)
	}
	err = collectRoute1(netlink.FAMILY_V4, ncWideTableID, linkIndex, subnets)
	if err != nil {
		return nil, fmt.Errorf("failed to collect route %d %d %d: %w", netlink.FAMILY_V4, ncWideTableID, linkIndex, err)
	}

	err = collectRoute1(netlink.FAMILY_V6, ncNarrowTableID, linkIndex, subnets)
	if err != nil {
		return nil, fmt.Errorf("failed to collect route %d %d %d: %w", netlink.FAMILY_V6, ncNarrowTableID, linkIndex, err)
	}
	err = collectRoute1(netlink.FAMILY_V6, ncWideTableID, linkIndex, subnets)
	if err != nil {
		return nil, fmt.Errorf("failed to collect route %d %d %d: %w", netlink.FAMILY_V6, ncWideTableID, linkIndex, err)
	}

	return subnets, nil
}

func collectRoute1(family, tableID, linkIndex int, subnets map[string]netlink.Route) error {
	routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: tableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: route list failed: %w", err)
	}
	for _, r := range routes {
		if r.LinkIndex == linkIndex && r.Dst != nil {
			subnets[r.Dst.String()] = r
		}
	}
	return nil
}

func subnetsSet(subnets []*net.IPNet) map[string]struct{} {
	subnetsSet := make(map[string]struct{})
	for _, subnet := range subnets {
		subnetsSet[subnet.String()] = struct{}{}
	}
	return subnetsSet
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

	// link up here to minimize the down time
	// See https://github.com/cybozu-go/coil/issues/287.
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("netlink: failed to link up %s: %w", link.Attrs().Name, err)
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
			return fmt.Errorf("netlink: failed to add route(table %d) to %s: %w", ncNarrowTableID, n.String(), err)
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
		return fmt.Errorf("netlink: failed to add route(table %d) to %s: %w", ncWideTableID, n.String(), err)
	}
	return nil
}

func configureRoutes(family iptables.Protocol) error {
	netlinkFamily := netlink.FAMILY_V4
	if family == iptables.ProtocolIPv6 {
		netlinkFamily = netlink.FAMILY_V6
	}

	links, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("netlink: failed to list links: %w", err)
	}

	ipt, err := iptables.New(iptables.IPFamily(family))
	if err != nil {
		return err
	}

	for _, link := range links {
		hasGlobalIP, err := checkLinkForGlobalScopeIP(link, netlinkFamily)
		if err != nil {
			return fmt.Errorf("netlink: failed to check interface %q for non-local IP address: %w", link.Attrs().Name, err)
		}
		if !hasGlobalIP {
			continue
		}

		linkTable := nonEgressTableID + link.Attrs().Index

		routes, err := netlink.RouteList(link, netlinkFamily)
		if err != nil {
			return fmt.Errorf("netlink: failed to list routes for interface %q: %w", link.Attrs().Name, err)
		}

		if len(routes) > 0 {
			for i := range routes {
				routes[i].Table = linkTable
				if err := netlink.RouteAdd(&routes[i]); err != nil && !errors.Is(err, syscall.EEXIST) {
					return fmt.Errorf("netlink: failed to add route %q, link: %s, state: %s - %w",
						routes[i].String(), link.Attrs().Name, link.Attrs().OperState.String(), err)
				}
			}

			if err := addFWMarkRule(link, linkTable, netlinkFamily); err != nil {
				return fmt.Errorf("failed to add FWMark rule: %w", err)
			}

			if err := addIPTRule(ipt, "mangle", "INPUT",
				"-m", "conntrack", "--ctstate", "NEW,ESTABLISHED,RELATED", "-j", "CONNMARK",
				"-i", link.Attrs().Name, "--set-mark", fmt.Sprintf("%d", link.Attrs().Index)); err != nil {
				return fmt.Errorf("failed to configure IPTables: %w", err)
			}

			if err := addIPTRule(ipt, "mangle", "OUTPUT", "-j", "CONNMARK", "-m", "connmark",
				"--mark", fmt.Sprintf("%d", link.Attrs().Index), "--restore-mark"); err != nil {
				return fmt.Errorf("failed to configure IPTables: %w", err)
			}
		}
	}
	return nil
}

func checkLinkForGlobalScopeIP(link netlink.Link, family int) (bool, error) {
	addrs, err := netlink.AddrList(link, family)
	if err != nil {
		return false, fmt.Errorf("netlink: failed to list addresses for linl %q: %w", link.Attrs().Name, err)
	}
	for _, a := range addrs {
		if a.Scope == int(netlink.SCOPE_UNIVERSE) {
			return true, nil
		}
	}
	return false, nil
}

func addFWMarkRule(link netlink.Link, table, family int) error {
	exists, err := checkFWMarkRule(link, table, family)
	if err != nil {
		return fmt.Errorf("failed to check FW mark rule existance: %w", err)
	}

	if !exists {
		rule := netlink.NewRule()
		rule.Mark = uint32(link.Attrs().Index)
		rule.Table = table
		rule.Family = family
		if err := netlink.RuleAdd(rule); err != nil {
			return fmt.Errorf("netlink: failed to add rule %q: %w", rule.String(), err)
		}
	}

	return nil
}

func checkFWMarkRule(link netlink.Link, table, family int) (bool, error) {
	rules, err := netlink.RuleList(family)
	if err != nil {
		return false, fmt.Errorf("netlink: failed to list rules: %w", err)
	}

	for _, r := range rules {
		if r.Mark == uint32(link.Attrs().Index) && r.Table == table {
			// rule already exists
			return true, nil
		}
	}
	return false, nil
}

func checkIPTRules(ipt *iptables.IPTables, table string, chain string, rulespec ...string) (bool, error) {
	exists, err := ipt.Exists(table, chain, rulespec...)
	if err != nil {
		return false, fmt.Errorf("failed to check %q rule in chain %q - %q: %w", table, chain, rulespec, err)
	}
	return exists, nil
}

func addIPTRule(ipt *iptables.IPTables, table string, chain string, rulespec ...string) error {
	exists, err := checkIPTRules(ipt, table, chain, rulespec...)
	if err != nil {
		return err
	}

	if !exists {
		if err := ipt.Append(table, chain, rulespec...); err != nil {
			return fmt.Errorf("failed to append %q rule in chain %q - %q: %w", table, chain, rulespec, err)
		}
	}

	return nil
}
