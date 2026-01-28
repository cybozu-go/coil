package netfilter

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"sync"
	"syscall"

	"github.com/vishvananda/netlink"

	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/nat"
)

// rule priorities
const (
	ncFWMarkPrio    = 1000
	ncLinkLocalPrio = 1800
	ncNarrowPrio    = 1900
	ncLocalPrioBase = 2000
	ncWidePrio      = 2100
)

// Table IDs
const (
	ncProtocolID    = 30
	ncNarrowTableID = 117
	ncWideTableID   = 118

	mainTableID      = 254
	nonEgressTableID = 1000
)

const (
	mangleTable = "mangle"
	inputChain  = "INPUT"
	outputChain = "OUTPUT"
)

var _ nat.Client = &NatClient{}

var (
	defaultIPv4GW = &net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)}
	defaultIPv6GW = &net.IPNet{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)}
)

var (
	v4PrivateList = []*net.IPNet{
		{IP: net.ParseIP("10.0.0.0").To4(), Mask: net.CIDRMask(8, 32)},
		{IP: net.ParseIP("172.16.0.0").To4(), Mask: net.CIDRMask(12, 32)},
		{IP: net.ParseIP("192.168.0.0").To4(), Mask: net.CIDRMask(16, 32)},
	}
	v6PrivateList = []*net.IPNet{
		{IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},
	}
)

var (
	v4LinkLocal = &net.IPNet{IP: net.ParseIP("169.254.0.0"), Mask: net.CIDRMask(16, 32)}
	v6LinkLocal = &net.IPNet{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)}
)

type NatClient struct {
	ipv4        net.IP
	ipv6        net.IP
	v4InCluster []*net.IPNet
	v6InCluster []*net.IPNet
	backend     string
	logFunc     func(string)

	mu sync.Mutex
}

func NewNatClient(ipv4, ipv6 net.IP, podNodeNet []*net.IPNet, backend string, logFunc func(string)) *NatClient {
	var v4InCluster, v6InCluster []*net.IPNet
	if len(podNodeNet) > 0 {
		for _, n := range podNodeNet {
			if n.IP.To4() != nil {
				v4InCluster = append(v4InCluster, n)
			} else {
				v6InCluster = append(v6InCluster, n)
			}
		}
	} else {
		v4InCluster = v4PrivateList
		v6InCluster = v6PrivateList
	}

	nc := &NatClient{
		ipv4:        ipv4,
		ipv6:        ipv6,
		v4InCluster: v4InCluster,
		v6InCluster: v6InCluster,
		backend:     backend,
		logFunc:     logFunc,
	}
	return nc
}

func (n *NatClient) Init() error {
	if n.ipv4 != nil {
		if err := n.clear(netlink.FAMILY_V4); err != nil {
			return fmt.Errorf("failed to clear IPv4 table: %v", err)
		}
		if err := n.initRules(netlink.FAMILY_V4); err != nil {
			return fmt.Errorf("failed to Init IPv4 table: %v", err)
		}
	}
	if n.ipv6 != nil {
		if err := n.clear(netlink.FAMILY_V6); err != nil {
			return fmt.Errorf("failed to clear IPv6 table: %v", err)
		}
		if err := n.initRules(netlink.FAMILY_V6); err != nil {
			return fmt.Errorf("failed to Init IPv6 table: %v", err)
		}
	}
	return nil
}

func (n *NatClient) IsInitialized() (bool, error) {
	if n.ipv4 != nil {
		if ok, err := isRuleInitialized(netlink.FAMILY_V4, n.v4InCluster); err != nil {
			return false, fmt.Errorf("failed to check IPv4 rule initialization: %w", err)
		} else if !ok {
			return false, err
		}
	}

	if n.ipv6 != nil {
		if ok, err := isRuleInitialized(netlink.FAMILY_V6, n.v6InCluster); err != nil {
			return false, fmt.Errorf("failed to check IPv6 rule initialization: %w", err)
		} else if !ok {
			return false, err
		}
	}

	return true, nil
}

func (n *NatClient) SyncNat(link netlink.Link, subnets []*net.IPNet, originatingOnly bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	routesBySubnet, err := collectRoutesBySubnet(link.Attrs().Index)
	if err != nil {
		return err
	}

	adds, dels := diffRoutes(routesBySubnet, subnets)

	for _, r := range dels {
		if err := n.deleteRoute(r); err != nil {
			return err
		}
	}

	configuredFamilies := make(map[int]struct{})
	for _, ipn := range adds {
		if err := n.addRoute(link, ipn); err != nil {
			return err
		}

		f := netlink.FAMILY_V4
		if ipn.IP.To4() == nil {
			f = netlink.FAMILY_V6
		}
		configuredFamilies[f] = struct{}{}
	}

	if originatingOnly {
		for f := range maps.Keys(configuredFamilies) {
			if err := configureOriginatingOnly(f, n.backend); err != nil {
				return err
			}
		}
	} else {
		for _, f := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
			if err := removeOriginatingOnly(f, n.backend); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *NatClient) initRules(family int) error {
	var linkLocal *net.IPNet
	switch family {
	case netlink.FAMILY_V4:
		linkLocal = v4LinkLocal
	case netlink.FAMILY_V6:
		linkLocal = v6LinkLocal
	}

	linkLocalRule := newRule(family, mainTableID, ncLinkLocalPrio)
	linkLocalRule.Dst = linkLocal
	if err := netlink.RuleAdd(linkLocalRule); err != nil && !errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("netlink: failed to add link local rule: %w", err)
	}

	narrowRule := newRule(family, ncNarrowTableID, ncNarrowPrio)
	if err := netlink.RuleAdd(narrowRule); err != nil && !errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("netlink: failed to add narrow rule: %w", err)
	}

	wideRule := newRule(family, ncWideTableID, ncWidePrio)
	if err := netlink.RuleAdd(wideRule); err != nil && !errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("netlink: failed to add wide rule: %w", err)
	}

	var inCluster []*net.IPNet
	switch family {
	case netlink.FAMILY_V4:
		inCluster = n.v4InCluster
	case netlink.FAMILY_V6:
		inCluster = n.v6InCluster
	}

	for i, ipn := range inCluster {
		r := newRule(family, mainTableID, ncLocalPrioBase+i)
		r.Dst = ipn
		if err := netlink.RuleAdd(r); err != nil && !errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("netlink: failed to add %s to rule: %w", ipn.String(), err)
		}
	}
	return nil
}

func (n *NatClient) clear(family int) error {
	var gw *net.IPNet
	switch family {
	case netlink.FAMILY_V4:
		gw = defaultIPv4GW
	case netlink.FAMILY_V6:
		gw = defaultIPv6GW
	default:
		return fmt.Errorf("invalid family: %d", family)
	}

	if err := n.clearRules(family, gw); err != nil {
		return err
	}

	if err := n.clearRoutes(family, gw); err != nil {
		return err
	}

	return nil
}

func (n *NatClient) clearRules(family int, gw *net.IPNet) error {
	rules, err := netlink.RuleList(family)
	if err != nil {
		return fmt.Errorf("netlink: rule list failed: %w", err)
	}

	for _, r := range rules {
		// skip non-NatClient rules
		if r.Priority < ncLinkLocalPrio || r.Priority > ncWidePrio {
			continue
		}
		if r.Dst == nil {
			// workaround for a library issue
			r.Dst = gw
		}

		if err := netlink.RuleDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a rule: %+v, %w", r, err)
		}
	}
	return nil
}

func (n *NatClient) clearRoutes(family int, gw *net.IPNet) error {
	for _, t := range [2]int{ncNarrowTableID, ncWideTableID} {
		routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: t}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return fmt.Errorf("netlink: route list failed: %w", err)
		}

		for _, r := range routes {
			if r.Dst == nil {
				// workaround for a library issue
				r.Dst = gw
			}

			if err := netlink.RouteDel(&r); err != nil {
				return fmt.Errorf("netlink: failed to delete a route in table %d: %+v, %w", ncNarrowTableID, r, err)
			}
		}
	}
	return nil
}

func isRuleInitialized(family int, inCluster []*net.IPNet) (bool, error) {
	rules, err := netlink.RuleListFiltered(family, &netlink.Rule{Table: mainTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return false, fmt.Errorf("netlink: failed to list main rules: %w", err)
	}
	// Ensure that all inCluster rules exist
	if len(rules) < (len(inCluster) + 1) {
		return false, nil
	}

	rules, err = netlink.RuleListFiltered(family, &netlink.Rule{Table: ncNarrowTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return false, fmt.Errorf("netlink: failed to list narrow rules: %w", err)
	}
	if len(rules) != 1 {
		return false, nil
	}

	rules, err = netlink.RuleListFiltered(family, &netlink.Rule{Table: ncWideTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return false, fmt.Errorf("netlink: failed to list wide rules: %w", err)
	}
	if len(rules) != 1 {
		return false, nil
	}
	return true, nil
}

func (n *NatClient) addRoute(link netlink.Link, ipn *net.IPNet) error {
	var inCluster []*net.IPNet
	if ipn.IP.To4() != nil {
		if n.ipv4 == nil {
			return nil
		}
		inCluster = n.v4InCluster
	} else {
		if n.ipv6 == nil {
			return nil
		}
		inCluster = n.v6InCluster
	}

	// link up here to minimize the down time
	// See https://github.com/cybozu-go/coil/issues/287.
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("netlink: failed to link up %s: %w", link.Attrs().Name, err)
	}

	for _, cipn := range inCluster {
		if !cipn.Contains(ipn.IP) {
			continue
		}

		if err := netlink.RouteAdd(&netlink.Route{
			Table:     ncNarrowTableID,
			Dst:       ipn,
			LinkIndex: link.Attrs().Index,
			Protocol:  ncProtocolID,
		}); err != nil {
			return fmt.Errorf("netlink: failed to add route(table %d) to %s: %w", ncNarrowTableID, ipn.String(), err)
		}
		return nil
	}

	if err := netlink.RouteAdd(&netlink.Route{
		Table:     ncWideTableID,
		Dst:       ipn,
		LinkIndex: link.Attrs().Index,
		Protocol:  ncProtocolID,
	}); err != nil {
		return fmt.Errorf("netlink: failed to add route(table %d) to %s: %w", ncWideTableID, ipn.String(), err)
	}
	return nil
}

func (n *NatClient) deleteRoute(r *netlink.Route) error {
	if n.logFunc != nil {
		n.logFunc(fmt.Sprintf("removing a destination %s", r.Dst.String()))
	}

	if err := netlink.RouteDel(r); err != nil {
		return fmt.Errorf("netlink: failed to delete a route  %+v, %w", r, err)
	}
	return nil
}

func collectRoutesBySubnet(linkIndex int) (map[string]netlink.Route, error) {
	routes := make(map[string]netlink.Route)
	for _, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		for _, table := range []int{ncNarrowTableID, ncWideTableID} {
			res, err := collectRoutesFromTable(family, table, linkIndex)
			if err != nil {
				return nil, fmt.Errorf("failed to collect route %d %d %d: %w", family, table, linkIndex, err)
			}
			maps.Copy(routes, res)
		}
	}
	return routes, nil
}

func collectRoutesFromTable(family, table, linkIndex int) (map[string]netlink.Route, error) {
	res := make(map[string]netlink.Route)
	rs, err := netlink.RouteListFiltered(family, &netlink.Route{Table: table}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return nil, fmt.Errorf("netlink: route list failed: %w", err)
	}

	for _, r := range rs {
		if r.LinkIndex == linkIndex && r.Dst != nil {
			res[r.Dst.String()] = r
		}
	}
	return res, nil
}

func diffRoutes(routes map[string]netlink.Route, subnets []*net.IPNet) (adds []*net.IPNet, dels []*netlink.Route) {
	rs := make(map[string]netlink.Route)
	maps.Copy(rs, routes)
	for _, subnet := range subnets {
		if _, ok := rs[subnet.String()]; !ok {
			adds = append(adds, subnet)
		} else {
			delete(rs, subnet.String())
		}
	}
	for _, r := range rs {
		dels = append(dels, &r)
	}
	return
}

func configureOriginatingOnly(family int, backend string) error {
	links, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("netlink: failed to list links: %w", err)
	}

	for _, link := range links {
		hasGlobalIP, err := checkLinkForGlobalScopeIP(link, family)
		if err != nil {
			return fmt.Errorf("netlink: failed to check interface %q for non-local IP address: %w", link.Attrs().Name, err)
		} else if !hasGlobalIP {
			continue
		}

		routes, err := netlink.RouteList(link, family)
		if err != nil {
			return fmt.Errorf("netlink: failed to list routes for interface %q: %w", link.Attrs().Name, err)
		} else if len(routes) == 0 {
			continue
		}

		linkTable := nonEgressTableID + link.Attrs().Index
		for i := range routes {
			routes[i].Table = linkTable
			if err := netlink.RouteAdd(&routes[i]); err != nil && !errors.Is(err, syscall.EEXIST) {
				return fmt.Errorf("netlink: failed to add route %q, link: %s, state: %s - %w",
					routes[i].String(), link.Attrs().Name, link.Attrs().OperState.String(), err)
			}
		}

		if err := addFWMarkRuleIfNotExists(link, linkTable, family); err != nil {
			return fmt.Errorf("failed to add FWMark rule: %w", err)
		}

		switch backend {
		case constants.EgressBackendIPTables:
			if err := setIPTablesConnmarkRules(family, link); err != nil {
				return err
			}
		case constants.EgressBackendNFTables:
			if err := setNFTablesConnmarkRules(family, link); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeOriginatingOnly(family int, backend string) error {
	links, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("netlink: failed to list links: %w", err)
	}

	for _, link := range links {
		hasGlobalIP, err := checkLinkForGlobalScopeIP(link, family)
		if err != nil {
			return fmt.Errorf("netlink: failed to check interface %q for non-local IP address: %w", link.Attrs().Name, err)
		} else if !hasGlobalIP {
			continue
		}

		linkTable := nonEgressTableID + link.Attrs().Index
		routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: linkTable}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return fmt.Errorf("netlink: failed to list routes for interface %q in table %q: %w", link.Attrs().Name, linkTable, err)
		} else if len(routes) == 0 {
			continue
		}

		for _, route := range routes {
			if err := netlink.RouteDel(&route); err != nil && !errors.Is(err, syscall.EEXIST) {
				return fmt.Errorf("netlink: failed to delete route %q, link: %s, state: %s - %w",
					route.String(), link.Attrs().Name, link.Attrs().OperState.String(), err)
			}
		}

		if err := delFWMarkRule(link, linkTable, family); err != nil {
			return fmt.Errorf("failed to delete FWMark rule: %w", err)
		}

		switch backend {
		case constants.EgressBackendIPTables:
			if err := removeIPTablesConnmarkRules(family, link); err != nil {
				return err
			}
		case constants.EgressBackendNFTables:
			if err := removeNFTablesConnmarkRules(family, link); err != nil {
				return err
			}
		}
	}
	return nil
}

func newRule(family, table, prio int) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = family
	r.Table = table
	r.Priority = prio
	return r
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

func addFWMarkRuleIfNotExists(link netlink.Link, table, family int) error {
	_, exists, err := checkFWMarkRule(link, table, family)
	if err != nil {
		return fmt.Errorf("failed to check FW mark rule existance: %w", err)
	} else if exists {
		return nil
	}

	rule := newRule(family, table, ncFWMarkPrio)
	rule.Mark = uint32(link.Attrs().Index)
	if err := netlink.RuleAdd(rule); err != nil {
		return fmt.Errorf("netlink: failed to add rule %q: %w", rule.String(), err)
	}

	return nil
}

func delFWMarkRule(link netlink.Link, table, family int) error {
	r, exists, err := checkFWMarkRule(link, table, family)
	if err != nil {
		return fmt.Errorf("failed to check FW mark rule existance: %w", err)
	} else if !exists {
		return nil
	}

	if err := netlink.RuleDel(r); err != nil {
		return fmt.Errorf("failed to remove FW mark rule: %w", err)
	}

	return nil
}

func checkFWMarkRule(link netlink.Link, table, family int) (*netlink.Rule, bool, error) {
	filter := &netlink.Rule{
		Priority: ncFWMarkPrio,
	}
	rules, err := netlink.RuleListFiltered(family, filter, netlink.RT_FILTER_PRIORITY)
	if err != nil {
		return nil, false, fmt.Errorf("netlink: failed to list rules: %w", err)
	}

	for _, r := range rules {
		if r.Mark == uint32(link.Attrs().Index) && r.Table == table {
			// rule already exists
			return &r, true, nil
		}
	}
	return nil, false, nil
}
