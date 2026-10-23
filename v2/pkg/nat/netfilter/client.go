package netfilter

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"sort"
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

// MaxInClusterNetworks is the maximum number of in-cluster networks (per IP family)
// that NatClient can install rules for. Each network gets its own rule priority
// starting at ncLocalPrioBase, and they must not collide with ncWidePrio.
const MaxInClusterNetworks = ncWidePrio - ncLocalPrioBase

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
	ipv4      net.IP
	ipv6      net.IP
	inCluster *ClusterNetworks
	backend   string
	logFunc   func(string)

	mu sync.Mutex
}

// NewNatClient creates a new NatClient.
func NewNatClient(ipv4, ipv6 net.IP, clusterNetworks *ClusterNetworks, backend string, logFunc func(string)) (*NatClient, error) {
	inCluster := &ClusterNetworks{v4PrivateList, v6PrivateList}

	if clusterNetworks != nil {
		if err := clusterNetworks.Validate(); err != nil {
			return nil, fmt.Errorf("failed to validate cluster-networks: %w", err)
		}
		if len(clusterNetworks.v4) > 0 {
			inCluster.v4 = clusterNetworks.v4
		}
		if len(clusterNetworks.v6) > 0 {
			inCluster.v6 = clusterNetworks.v6
		}
	}

	nc := &NatClient{
		ipv4:      ipv4,
		ipv6:      ipv6,
		inCluster: inCluster,
		backend:   backend,
		logFunc:   logFunc,
	}
	return nc, nil
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

	if err := n.SyncInClusterRules(); err != nil {
		return fmt.Errorf("failed to init in-CLuster rules: %w", err)
	}

	return nil
}

func (n *NatClient) IsInitialized() (bool, error) {
	if n.ipv4 != nil {
		if ok, err := isRuleInitialized(netlink.FAMILY_V4); err != nil {
			return false, fmt.Errorf("failed to check IPv4 rule initialization: %w", err)
		} else if !ok {
			return false, err
		}
	}

	if n.ipv6 != nil {
		if ok, err := isRuleInitialized(netlink.FAMILY_V6); err != nil {
			return false, fmt.Errorf("failed to check IPv6 rule initialization: %w", err)
		} else if !ok {
			return false, err
		}
	}

	return true, nil
}

func (n *NatClient) IsInClusterInitialized() (bool, error) {
	if n.ipv4 != nil {
		if ok, err := areInClusterRulesInitialized(netlink.FAMILY_V4, n.inCluster.v4); err != nil {
			return false, fmt.Errorf("failed to check IPv4 rule initialization: %w", err)
		} else if !ok {
			return false, err
		}
	}

	if n.ipv6 != nil {
		if ok, err := areInClusterRulesInitialized(netlink.FAMILY_V6, n.inCluster.v6); err != nil {
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

	families := make(map[int]struct{})
	if n.ipv4 != nil {
		families[netlink.FAMILY_V4] = struct{}{}
	}
	if n.ipv6 != nil {
		families[netlink.FAMILY_V6] = struct{}{}
	}

	subnetsV4 := []*net.IPNet{}
	subnetsV6 := []*net.IPNet{}

	for _, s := range subnets {
		if s.IP.To4() != nil {
			subnetsV4 = append(subnetsV4, s)
		} else {
			subnetsV6 = append(subnetsV6, s)
		}
	}

	for f := range families {
		s := subnetsV4
		if f == netlink.FAMILY_V6 {
			s = subnetsV6
		}

		routesBySubnet, err := collectRoutesBySubnet(link.Attrs().Index, f)
		if err != nil {
			return err
		}

		if err := n.syncNat(link, s, originatingOnly, routesBySubnet, f); err != nil {
			return err
		}
	}

	return nil
}

func (n *NatClient) syncNat(link netlink.Link, subnets []*net.IPNet, originatingOnly bool, routesBySubnet map[string]netlink.Route, family int) error {
	adds, dels, keeps := diffRoutes(routesBySubnet, subnets)

	for _, r := range dels {
		if err := n.deleteRoute(r); err != nil {
			return err
		}
	}

	added := false
	for _, ipn := range adds {
		if err := n.addRoute(link, ipn); err != nil {
			return err
		}
		added = true
	}

	if err := n.moveRoutes(keeps, family); err != nil {
		return err
	}

	if originatingOnly {
		if added {
			if err := configureOriginatingOnly(family, n.backend); err != nil {
				return err
			}
		}
	} else {
		if err := removeOriginatingOnly(family, n.backend); err != nil {
			return err
		}
	}
	return nil
}

func (n *NatClient) moveRoutes(routes []netlink.Route, family int) error {
	for _, r := range routes {
		// select proper IP family and cluster-networks
		cn := n.inCluster.v4
		if family == netlink.FAMILY_V6 {
			cn = n.inCluster.v6
		}

		expectedTable := ncWideTableID
		for _, subnet := range cn {
			if ipnetContains(subnet, r.Dst) {
				// at least one cluster-network subnet contains the egress route subnet
				// which means egress subnet should be 'narrow'
				expectedTable = ncNarrowTableID
				break
			}
		}

		if r.Table == expectedTable {
			// existing route is in a proper table, do nothing
			continue
		}

		// add the rule to the correct table
		if err := netlink.RouteAdd(&netlink.Route{
			Table:     expectedTable,
			Dst:       r.Dst,
			LinkIndex: r.LinkIndex,
			Protocol:  ncProtocolID,
		}); err != nil {
			return fmt.Errorf("netlink: failed to add route %q: %w", r.Dst, err)
		}

		// delete route from the incorrect table
		if err := netlink.RouteDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete route %q: %w", r.Dst, err)
		}

	}

	return nil
}

func ipnetContains(a, b *net.IPNet) bool {
	if (a.IP.To4() == nil) != (b.IP.To4() == nil) {
		return false
	}
	onesA, _ := a.Mask.Size()
	onesB, _ := b.Mask.Size()
	return onesA <= onesB && a.Contains(b.IP)
}

func (n *NatClient) SyncInClusterRules() error {
	if n.ipv4 != nil {
		if err := n.syncInClusterRules(netlink.FAMILY_V4, n.inCluster.v4); err != nil {
			return fmt.Errorf("failed to sync IPv4 in-cluster rules: %w", err)
		}
	}
	if n.ipv6 != nil {
		if err := n.syncInClusterRules(netlink.FAMILY_V6, n.inCluster.v6); err != nil {
			return fmt.Errorf("failed to sync IPv6 in-cluster rules: %w", err)
		}
	}
	return nil
}

func (n *NatClient) syncInClusterRules(family int, inCluster []*net.IPNet) error {
	rules, err := netlink.RuleListFiltered(family, &netlink.Rule{
		Table: mainTableID,
	}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("failed to list rules: %w", err)
	}

	existingRules := make([]*netlink.Rule, ncWidePrio-ncLocalPrioBase)
	existingRulesMap := make(map[string]*netlink.Rule)

	for i, r := range rules {
		if r.Priority >= ncLocalPrioBase && r.Priority < ncWidePrio {
			rDst := r.Dst
			if rDst == nil {
				rDst = getDefaultGlobalDst(family)
			}
			existingRules[r.Priority-ncLocalPrioBase] = &rules[i]
			existingRulesMap[rDst.String()] = &rules[i]
		}
	}

	candidatesMap := make(map[string]*net.IPNet)
	for i := range inCluster {
		candidatesMap[inCluster[i].String()] = inCluster[i]
	}

	toDelete := []*netlink.Rule{}
	for addr, r := range existingRulesMap {
		if _, exists := candidatesMap[addr]; !exists {
			toDelete = append(toDelete, r)
		}
	}

	toAdd := []*net.IPNet{}

	for addr, ipn := range candidatesMap {
		if _, exists := existingRulesMap[addr]; !exists {
			toAdd = append(toAdd, ipn)
		}
	}

	sort.Slice(toAdd, func(i, j int) bool {
		return toAdd[i].String() < toAdd[j].String()
	})

	for _, r := range toDelete {
		if err := netlink.RuleDel(r); err != nil {
			return fmt.Errorf("netlink: failed to delete rule: %q: %w", r.String(), err)
		}
		existingRules[r.Priority-ncLocalPrioBase] = nil
	}

	index := 0
	for _, ipn := range toAdd {
		for ; index < len(existingRules); index++ {
			if existingRules[index] == nil {
				break
			}
		}
		if index >= len(existingRules) {
			return fmt.Errorf("no free rule priorities available")
		}

		r := newRule(family, mainTableID, ncLocalPrioBase+index)
		r.Dst = ipn
		if err := netlink.RuleAdd(r); err != nil && !errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("netlink: failed to add rule %q: %w", r.String(), err)
		}
		index++
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

func isRuleInitialized(family int) (bool, error) {
	rules, err := netlink.RuleListFiltered(family, &netlink.Rule{Table: ncNarrowTableID}, netlink.RT_FILTER_TABLE)
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

func areInClusterRulesInitialized(family int, inCluster []*net.IPNet) (bool, error) {
	rules, err := netlink.RuleListFiltered(family, &netlink.Rule{Table: mainTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return false, fmt.Errorf("netlink: failed to list main rules: %w", err)
	}

	// Build local lookup map
	existingRules := make(map[string]struct{}, len(rules))
	for _, r := range rules {
		dst := r.Dst
		if dst == nil {
			dst = getDefaultGlobalDst(family)
		}

		existingRules[dst.String()] = struct{}{}
	}

	missing := []string{}
	// Ensure that all inCluster rules exist
	for _, ipn := range inCluster {
		if _, ok := existingRules[ipn.String()]; !ok {
			missing = append(missing, ipn.String())
		}
	}

	if len(missing) > 0 {
		return false, fmt.Errorf("missing in-cluster rules for subnets %v", missing)
	}

	return true, nil
}

func getDefaultGlobalDst(family int) *net.IPNet {
	if family == netlink.FAMILY_V6 {
		return defaultIPv6GW
	}
	return defaultIPv4GW
}

func (n *NatClient) addRoute(link netlink.Link, ipn *net.IPNet) error {
	var inCluster []*net.IPNet
	if ipn.IP.To4() != nil {
		if n.ipv4 == nil {
			return nil
		}
		inCluster = n.inCluster.v4
	} else {
		if n.ipv6 == nil {
			return nil
		}
		inCluster = n.inCluster.v6
	}

	// link up here to minimize the down time
	// See https://github.com/cybozu-go/coil/issues/287.
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("netlink: failed to link up %s: %w", link.Attrs().Name, err)
	}

	for _, cipn := range inCluster {
		if !ipnetContains(cipn, ipn) {
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

func collectRoutesBySubnet(linkIndex int, family int) (map[string]netlink.Route, error) {
	routes := make(map[string]netlink.Route)
	for _, table := range []int{ncNarrowTableID, ncWideTableID} {
		res, err := collectRoutesFromTable(family, table, linkIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to collect route %d %d %d: %w", family, table, linkIndex, err)
		}

		maps.Copy(routes, res)

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
		if r.LinkIndex == linkIndex {
			res[r.Dst.String()] = r
		}
	}
	return res, nil
}

func diffRoutes(routes map[string]netlink.Route, subnets []*net.IPNet) (adds []*net.IPNet, dels []*netlink.Route, keeps []netlink.Route) {
	rs := make(map[string]netlink.Route)
	maps.Copy(rs, routes)
	for _, subnet := range subnets {
		if r, ok := rs[subnet.String()]; !ok {
			adds = append(adds, subnet)
		} else {
			keeps = append(keeps, r)
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
			return fmt.Errorf("netlink: failed to list routes for interface %q in table %d: %w", link.Attrs().Name, linkTable, err)
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
			if err := removeNFTablesConnmarkRules(family); err != nil {
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
		return false, fmt.Errorf("netlink: failed to list addresses for link %q: %w", link.Attrs().Name, err)
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
		return fmt.Errorf("failed to check FW mark rule existence: %w", err)
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
		return fmt.Errorf("failed to check FW mark rule existence: %w", err)
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
