package founat

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"testing"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/coreos/go-iptables/iptables"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/vishvananda/netlink"
)

func TestClient(t *testing.T) {
	t.Run("Default", testClientDual)
	t.Run("IPv4", testClientV4)
	t.Run("IPv6", testClientV6)
	t.Run("Custom", testClientCustom)
}

func ruleMap(family int) (map[int]*netlink.Rule, error) {
	rules, err := netlink.RuleList(family)
	if err != nil {
		return nil, err
	}
	m := make(map[int]*netlink.Rule)
	for _, r := range rules {
		r := r
		m[r.Priority] = &r
	}
	return m, nil
}

func testClientDual(t *testing.T) {
	t.Parallel()
	for _, backend := range []string{constants.EgressBackendIPTables, constants.EgressBackendNFTables} {
		cNS, err := ns.GetNS(fmt.Sprintf("/run/netns/test-client-dual-%s", backend))
		if err != nil {
			t.Fatal(err)
		}
		defer cNS.Close()

		err = cNS.Do(func(ns.NetNS) error {
			return testClientDualFunc(false, backend)
		})
		if err != nil {
			t.Error(err)
		}

		coNS, err := ns.GetNS(fmt.Sprintf("/run/netns/test-client-dual-%s-oo", backend))
		if err != nil {
			t.Fatal(err)
		}
		defer coNS.Close()

		err = coNS.Do(func(ns.NetNS) error {
			return testClientDualFunc(true, backend)
		})
		if err != nil {
			t.Error(err)
		}
	}
}

func testClientDualFunc(originatingOnly bool, backend string) error {
	nc := NewNatClient(net.ParseIP("10.1.1.1"), net.ParseIP("fd02::1"), nil, backend, nil)
	initialized, err := nc.IsInitialized()
	if err != nil {
		return err
	}
	if initialized {
		return errors.New("expect not to be initialized, but it's already been done")
	}

	if err := nc.Init(); err != nil {
		return err
	}

	rm, err := ruleMap(netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	if r, ok := rm[1800]; !ok {
		return errors.New("no ipv4 link local rule")
	} else {
		if r.Table != 254 {
			return errors.New("link local rule should point the main table")
		}
	}
	if r, ok := rm[1900]; !ok {
		return errors.New("no ipv4 narrow rule")
	} else {
		if r.Table != 117 {
			return errors.New("narrow rule should point routing table 117")
		}
	}
	if r, ok := rm[2000]; !ok {
		return errors.New("no rule for IPv4 private networks")
	} else {
		if r.Table != 254 {
			return errors.New("private network rule should point the main table")
		}
	}
	if _, ok := rm[2002]; !ok {
		return errors.New("no rule exists for priority 2002")
	}
	if r, ok := rm[2100]; !ok {
		return errors.New("no ipv4 wide rule")
	} else {
		if r.Table != 118 {
			return errors.New("wide rule should point routing table 118")
		}
	}

	rm, err = ruleMap(netlink.FAMILY_V6)
	if err != nil {
		return err
	}
	if r, ok := rm[1800]; !ok {
		return errors.New("no ipv6 link local rule")
	} else {
		if r.Table != 254 {
			return errors.New("link local rule should point the main table")
		}
	}
	if r, ok := rm[1900]; !ok {
		return errors.New("no ipv6 narrow rule")
	} else {
		if r.Table != 117 {
			return errors.New("narrow rule should point routing table 117")
		}
	}
	if r, ok := rm[2000]; !ok {
		return errors.New("no rule for IPv6 private networks")
	} else {
		if r.Table != 254 {
			return errors.New("private network rule should point the main table")
		}
	}
	if r, ok := rm[2100]; !ok {
		return errors.New("no ipv6 wide rule")
	} else {
		if r.Table != 118 {
			return errors.New("wide rule should point routing table 118")
		}
	}

	initialized, err = nc.IsInitialized()
	if err != nil {
		return err
	}
	if !initialized {
		return errors.New("expect to be initialized, but it's not been done")
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = "dummy1"
	attrs.Flags = net.FlagUp
	dummy := &netlink.Dummy{LinkAttrs: attrs}
	if err := netlink.LinkAdd(dummy); err != nil {
		return fmt.Errorf("failed to add dummy link: %w", err)
	}
	link, err := netlink.LinkByName("dummy1")
	if err != nil {
		return fmt.Errorf("failed to get dummy1: %w", err)
	}
	err = nc.AddEgress(link, []*net.IPNet{
		{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)},
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
	}, originatingOnly)
	if err != nil {
		return fmt.Errorf("failed to add egress: %w", err)
	}

	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 1 {
		return errors.New("failed to add ipv4 dst to table 117")
	}
	if !routes[0].Dst.IP.Equal(net.ParseIP("10.1.2.0")) {
		return fmt.Errorf("wrong dst in table 117: %s", routes[0].Dst.String())
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 1 {
		return errors.New("failed to add ipv4 dst to table 118")
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}

	if len(routes) != 1 {
		return errors.New("failed to add ipv6 dst to table 117")
	}
	if !routes[0].Dst.IP.Equal(net.ParseIP("fd02::")) {
		return fmt.Errorf("wrong dst in table 117: %s", routes[0].Dst.String())
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 1 {
		return errors.New("failed to add ipv6 dst to table 118")
	}

	// Update the destinations
	err = nc.AddEgress(link, []*net.IPNet{
		{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("10.1.3.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("9.9.9.9"), Mask: net.CIDRMask(32, 32)},
		{IP: net.ParseIP("fd03::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("fd04::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("fd05::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
	}, originatingOnly)

	if err != nil {
		return fmt.Errorf("failed to add egress: %w", err)
	}

	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 2 {
		return errors.New("failed to update ipv4 dst in table 117")
	}
	expectedIPs := make(map[string]struct{})
	for _, route := range routes {
		expectedIPs[route.Dst.IP.String()] = struct{}{}
	}
	if _, ok := expectedIPs["10.1.2.0"]; !ok {
		return fmt.Errorf("wrong dst in table 117: 10.1.2.0 not included")
	}
	if _, ok := expectedIPs["10.1.3.0"]; !ok {
		return fmt.Errorf("wrong dst in table 117: 10.1.3.0 not included")
	}

	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 1 {
		return errors.New("failed to add ipv4 dst to table 118")
	}
	if !routes[0].Dst.IP.Equal(net.ParseIP("9.9.9.9")) {
		return fmt.Errorf("wrong dst in table 118: %s", routes[0].Dst.String())
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}

	if len(routes) != 3 {
		return errors.New("failed to update ipv6 dst in table 117")
	}
	expectedIPs = make(map[string]struct{})
	for _, route := range routes {
		expectedIPs[route.Dst.IP.String()] = struct{}{}
	}
	if _, ok := expectedIPs["fd03::"]; !ok {
		return fmt.Errorf("wrong dst in table 117: fd03:: not included")
	}
	if _, ok := expectedIPs["fd04::"]; !ok {
		return fmt.Errorf("wrong dst in table 117: fd04:: not included")
	}
	if _, ok := expectedIPs["fd05::"]; !ok {
		return fmt.Errorf("wrong dst in table 117: fd05:: not included")
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 1 {
		return errors.New("failed to add ipv6 dst to table 118")
	}
	// NATClient can be re-initialized
	if err := nc.Init(); err != nil {
		return fmt.Errorf("failed to re-initialize NATClient: %w", err)
	}

	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 0 {
		return fmt.Errorf("routing table 117 should be cleared for IPv4: %v", routes)
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 0 {
		return fmt.Errorf("routing table 118 should be cleared for IPv4: %v", routes)
	}

	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 0 {
		return fmt.Errorf("routing table 117 should be cleared for IPv6: %v", routes)
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 0 {
		return fmt.Errorf("routing table 118 should be cleared for IPv6: %v", routes)
	}

	if originatingOnly {
		addrs := []string{"100.100.100.100/24", "d0d5:1e73:46c3:d7a9:fa27:90e7:9540:d895/112"}
		testAddrs := []string{"10.1.10.0/24", "fd10::/112"}

		if err := testOriginatingOnly(link, nc, false, addrs, testAddrs, backend); err != nil {
			return fmt.Errorf("originatingOnly test with no IP addresses failed: %w", err)
		}

		addrs = []string{"100.100.101.100/24", "d0d5:1e73:46c3:d7a9:fa27:90e7:9541:d895/112"}
		testAddrs = []string{"10.1.11.0/24", "fd11::/16"}

		if err := testOriginatingOnly(link, nc, true, addrs, testAddrs, backend); err != nil {
			return fmt.Errorf("originatingOnly test with IP addresses failed: %w", err)
		}
	}

	return nil
}

func testClientV4(t *testing.T) {
	t.Parallel()

	for _, backend := range []string{constants.EgressBackendIPTables, constants.EgressBackendNFTables} {
		cNS, err := ns.GetNS(fmt.Sprintf("/run/netns/test-client-v4-%s", backend))
		if err != nil {
			t.Fatal(err)
		}
		defer cNS.Close()

		err = cNS.Do(func(ns.NetNS) error {
			return testClientV4Func(false, backend)
		})
		if err != nil {
			t.Error(err)
		}

		coNS, err := ns.GetNS(fmt.Sprintf("/run/netns/test-client-v4-%s-oo", backend))
		if err != nil {
			t.Fatal(err)
		}
		defer coNS.Close()

		err = coNS.Do(func(ns.NetNS) error {
			return testClientV4Func(true, backend)
		})
		if err != nil {
			t.Error(err)
		}
	}

}

func testClientV4Func(originatingOnly bool, backend string) error {
	nc := NewNatClient(net.ParseIP("10.1.1.1"), nil, nil, backend, nil)
	initialized, err := nc.IsInitialized()
	if err != nil {
		return err
	}
	if initialized {
		return errors.New("expect not to be initialized, but it's already been done")
	}

	if err := nc.Init(); err != nil {
		return err
	}

	rm, err := ruleMap(netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	if _, ok := rm[1800]; !ok {
		return errors.New("no ipv4 link local rule")
	}

	rm, err = ruleMap(netlink.FAMILY_V6)
	if err != nil {
		return err
	}
	if _, ok := rm[1800]; ok {
		return errors.New("ipv6 link local rule exists")
	}

	initialized, err = nc.IsInitialized()
	if err != nil {
		return err
	}
	if !initialized {
		return errors.New("expect to be initialized, but it's not been done")
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = "dummy1"
	attrs.Flags = net.FlagUp
	dummy := &netlink.Dummy{LinkAttrs: attrs}
	if err := netlink.LinkAdd(dummy); err != nil {
		return fmt.Errorf("failed to add dummy link: %w", err)
	}
	link, err := netlink.LinkByName("dummy1")
	if err != nil {
		return fmt.Errorf("failed to get dummy1: %w", err)
	}
	err = nc.AddEgress(link, []*net.IPNet{
		{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)},
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
	}, originatingOnly)
	if err != nil {
		return fmt.Errorf("failed to add egress: %w", err)
	}

	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 1 {
		return errors.New("failed to add ipv4 dst to table 117")
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 0 {
		return errors.New("ipv6 should be ignored")
	}

	if originatingOnly {
		addrs := []string{"100.100.100.100/24"}
		testAddrs := []string{"10.1.10.0/24"}
		if err := testOriginatingOnly(link, nc, false, addrs, testAddrs, backend); err != nil {
			return fmt.Errorf("originatingOnly test with no IP addresses failed: %w", err)
		}

		addrs = []string{"100.100.101.100/24"}
		testAddrs = []string{"10.1.11.0/24"}
		if err := testOriginatingOnly(link, nc, true, addrs, testAddrs, backend); err != nil {
			return fmt.Errorf("originatingOnly test with IP addresses failed: %w", err)
		}
	}

	return nil
}

func testClientV6(t *testing.T) {
	t.Parallel()
	for _, backend := range []string{constants.EgressBackendIPTables, constants.EgressBackendNFTables} {
		cNS, err := ns.GetNS(fmt.Sprintf("/run/netns/test-client-v6-%s", backend))
		if err != nil {
			t.Fatal(err)
		}
		defer cNS.Close()

		err = cNS.Do(func(ns.NetNS) error {
			return testClientV6Func(false, backend)
		})
		if err != nil {
			t.Error(err)
		}

		coNS, err := ns.GetNS(fmt.Sprintf("/run/netns/test-client-v6-%s-oo", backend))
		if err != nil {
			t.Fatal(err)
		}
		defer coNS.Close()

		err = coNS.Do(func(ns.NetNS) error {
			return testClientV6Func(true, backend)
		})
		if err != nil {
			t.Error(err)
		}
	}

}

func testClientV6Func(originatingOnly bool, backend string) error {
	nc := NewNatClient(nil, net.ParseIP("fd02::1"), nil, backend, nil)
	initialized, err := nc.IsInitialized()
	if err != nil {
		return err
	}
	if initialized {
		return errors.New("expect not to be initialized, but it's already been done")
	}

	if err := nc.Init(); err != nil {
		return err
	}

	rm, err := ruleMap(netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	if _, ok := rm[1800]; ok {
		return errors.New("ipv4 link local rule exists")
	}

	rm, err = ruleMap(netlink.FAMILY_V6)
	if err != nil {
		return err
	}
	if _, ok := rm[1800]; !ok {
		return errors.New("no ipv6 link local rule")
	}

	initialized, err = nc.IsInitialized()
	if err != nil {
		return err
	}
	if !initialized {
		return errors.New("expect to be initialized, but it's not been done")
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = "dummy1"
	attrs.Flags = net.FlagUp
	dummy := &netlink.Dummy{LinkAttrs: attrs}
	if err := netlink.LinkAdd(dummy); err != nil {
		return fmt.Errorf("failed to add dummy link: %w", err)
	}
	link, err := netlink.LinkByName("dummy1")
	if err != nil {
		return fmt.Errorf("failed to get dummy1: %w", err)
	}
	err = nc.AddEgress(link, []*net.IPNet{
		{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)},
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
	}, originatingOnly)
	if err != nil {
		return fmt.Errorf("failed to add egress: %w", err)
	}

	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 0 {
		return errors.New("ipv4 should be ignored")
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 1 {
		return errors.New("failed to add ipv6 dst to table 117")
	}

	if originatingOnly {
		addrs := []string{"d0d5:1e73:46c3:d7a9:fa27:90e7:9540:d895/112"}
		testAddrs := []string{"fd10::/16"}

		if err := testOriginatingOnly(link, nc, false, addrs, testAddrs, backend); err != nil {
			return fmt.Errorf("originatingOnly test with no IP addresses failed: %w", err)
		}

		addrs = []string{"d0d5:1e73:46c3:d7a9:fa27:90e7:9541:d895/112"}
		testAddrs = []string{"fd11::/16"}

		if err := testOriginatingOnly(link, nc, true, addrs, testAddrs, backend); err != nil {
			return fmt.Errorf("originatingOnly test with IP addresses failed: %w", err)
		}
	}

	return nil
}

func testClientCustom(t *testing.T) {
	t.Parallel()

	for _, backend := range []string{constants.EgressBackendIPTables, constants.EgressBackendNFTables} {
		cNS, err := ns.GetNS(fmt.Sprintf("/run/netns/test-client-custom-%s", backend))
		if err != nil {
			t.Fatal(err)
		}
		defer cNS.Close()

		err = cNS.Do(func(ns.NetNS) error {
			return testClientCustomFunc(false, backend)
		})
		if err != nil {
			t.Error(err)
		}

		coNS, err := ns.GetNS(fmt.Sprintf("/run/netns/test-client-custom-%s-oo", backend))
		if err != nil {
			t.Fatal(err)
		}
		defer coNS.Close()

		err = coNS.Do(func(ns.NetNS) error {
			return testClientCustomFunc(true, backend)
		})
		if err != nil {
			t.Error(err)
		}
	}

}

func testClientCustomFunc(originatingOnly bool, backend string) error {
	nc := NewNatClient(net.ParseIP("10.1.1.1"), net.ParseIP("fd02::1"), []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}, backend, nil)
	initialized, err := nc.IsInitialized()
	if err != nil {
		return err
	}
	if initialized {
		return errors.New("expect not to be initialized, but it's already been done")
	}

	if err := nc.Init(); err != nil {
		return err
	}

	initialized, err = nc.IsInitialized()
	if err != nil {
		return err
	}
	if !initialized {
		return errors.New("expect to be initialized, but it's not been done")
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = "dummy1"
	attrs.Flags = net.FlagUp
	dummy := &netlink.Dummy{LinkAttrs: attrs}
	if err := netlink.LinkAdd(dummy); err != nil {
		return fmt.Errorf("failed to add dummy link: %w", err)
	}
	link, err := netlink.LinkByName("dummy1")
	if err != nil {
		return fmt.Errorf("failed to get dummy1: %w", err)
	}
	err = nc.AddEgress(link, []*net.IPNet{
		{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("fd03::"), Mask: net.CIDRMask(64, 128)},
	}, originatingOnly)
	if err != nil {
		return fmt.Errorf("failed to add egress: %w", err)
	}

	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 0 {
		return errors.New("should respect custom IPv4 private networks")
	}
	routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	if len(routes) != 0 {
		return errors.New("should respect custom IPv6 private networks")
	}

	return nil
}

func testOriginatingOnly(link netlink.Link, nc NatClient, addAddresses bool, addrs, testAddrs []string, backend string) error {
	for i := range addrs {
		ip, ipnet, err := net.ParseCIDR(addrs[i])
		if err != nil {
			return fmt.Errorf("failed to parse CIDR %q: %w", addrs[i], err)
		}

		if addAddresses {
			if err := netlink.AddrAdd(link, &netlink.Addr{
				IPNet: netlink.NewIPNet(ip),
				Scope: int(netlink.SCOPE_UNIVERSE),
			}); err != nil {
				return fmt.Errorf("failed to add address %q: %w", ip.String(), err)
			}
		}

		family := netlink.FAMILY_V4
		if ip.To4() == nil {
			family = netlink.FAMILY_V6
		}

		egressLinkTable := nonEgressTableID + link.Attrs().Index

		if addAddresses {
			if err := netlink.RouteAdd(&netlink.Route{
				LinkIndex: link.Attrs().Index,
				Scope:     netlink.SCOPE_HOST,
				Dst:       ipnet,
			}); err != nil {
				return fmt.Errorf("failed to add route %q: %w", ipnet.String(), err)
			}
		}

		_, testIPNet, err := net.ParseCIDR(testAddrs[i])
		if err != nil {
			return fmt.Errorf("failed to parse CIDR %q: %w", testAddrs[i], err)
		}

		err = nc.AddEgress(link, []*net.IPNet{
			testIPNet,
		}, true)
		if err != nil {
			return fmt.Errorf("failed to add egress: %w", err)
		}

		routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}

		if len(routes) != 1 {
			return errors.New("failed to add ipv4 dst to table 117")
		}
		if !routes[0].Dst.IP.Equal(testIPNet.IP) {
			return fmt.Errorf("wrong dst in table 117: %s", routes[0].Dst.String())
		}

		originalRoutes, err := netlink.RouteList(link, family)
		if err != nil {
			return err
		}

		newRoutes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: egressLinkTable}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}

		if addAddresses {
			if len(originalRoutes) == len(newRoutes) {
				if slices.CompareFunc(originalRoutes, newRoutes, func(o netlink.Route, n netlink.Route) int {
					if o.Dst.IP.Equal(n.Dst.IP) && o.Table != n.Table {
						return 0
					}
					return 1
				}) != 0 {
					return fmt.Errorf("wrong values in table %d", egressLinkTable)
				}

				_, exists, err := checkFWMarkRule(link, egressLinkTable, family)
				if err != nil {
					return fmt.Errorf("failed while checking FW Mark rule: %w", err)
				}
				if !exists {
					return fmt.Errorf("FM mark rule does not exist")
				}

				iptFamily := iptables.ProtocolIPv4
				if family == netlink.FAMILY_V6 {
					iptFamily = iptables.ProtocolIPv6
				}

				ipt, err := iptables.New(iptables.IPFamily(iptFamily))
				if err != nil {
					return err
				}

				if backend == constants.EgressBackendIPTables {
					exists, err = checkIPTRules(ipt, "mangle", "INPUT",
						"-m", "conntrack", "--ctstate", "NEW,ESTABLISHED,RELATED", "-j", "CONNMARK",
						"-i", link.Attrs().Name, "--set-mark", fmt.Sprintf("%d", link.Attrs().Index))
					if err != nil {
						return fmt.Errorf("failed to check IPTables mangle INPUT rule: %w", err)
					}
					if !exists {
						return fmt.Errorf("expected IPTables mangle INPUT rule does not exist")
					}

					exists, err = checkIPTRules(ipt, "mangle", "OUTPUT", "-j", "CONNMARK", "-m", "connmark",
						"--mark", fmt.Sprintf("%d", link.Attrs().Index), "--restore-mark")
					if err != nil {
						return fmt.Errorf("failed to check IPTables mangle OUTPUT rule: %w", err)
					}
					if !exists {
						return fmt.Errorf("expected IPTables mangle OUTPUT rule does not exist")
					}
				} else {
					nft, err := nftables.New()
					if err != nil {
						return fmt.Errorf("failed to create nft connection: %w", err)
					}
					nfFamily := nftables.TableFamilyIPv4
					if family == netlink.FAMILY_V6 {
						nfFamily = nftables.TableFamilyIPv6
					}
					table, err := nft.ListTableOfFamily("mangle", nfFamily)
					if err != nil {
						return fmt.Errorf("failed to get mangle table: %w", err)
					}
					inputChain, err := nft.ListChain(table, "input")
					if err != nil {
						return fmt.Errorf("failed to get input chain in table %q: %w", table.Name, err)
					}

					outputChain, err := nft.ListChain(table, "output")
					if err != nil {
						return fmt.Errorf("failed to get output chain in table %q: %w", table.Name, err)
					}

					deviceSet, err := nft.GetSetByName(table, link.Attrs().Name)
					if err != nil {
						return fmt.Errorf("failed to get set for device %q: %w", link.Attrs().Name, err)
					}

					markRule := &nftables.Rule{
						Table: table,
						Chain: inputChain,
						Exprs: []expr.Any{
							&expr.Meta{
								Key:      expr.MetaKeyIIFNAME,
								Register: 1,
							},
							&expr.Lookup{
								SetName:        deviceSet.Name,
								SetID:          deviceSet.ID,
								SourceRegister: 1,
							},
							&expr.Ct{
								Register: 1,
								Key:      expr.CtKeySTATE,
							},
							&expr.Bitwise{
								SourceRegister: 1,
								DestRegister:   1,
								Len:            4,
								Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitNEW | expr.CtStateBitESTABLISHED | expr.CtStateBitRELATED),
								Xor:            binaryutil.NativeEndian.PutUint32(0),
							},
							&expr.Cmp{
								Op:       expr.CmpOpNeq,
								Register: 1,
								Data:     []byte{0, 0, 0, 0},
							},
							&expr.Counter{},
							&expr.Immediate{
								Register: 1,
								Data:     binaryutil.NativeEndian.PutUint32(uint32(link.Attrs().Index)),
							},
							&expr.Ct{
								Key:            expr.CtKeyMARK,
								SourceRegister: false,
								Register:       0,
							},
						},
					}

					exists, err := checkNftRule(nft, markRule)
					if err != nil {
						return fmt.Errorf("failed to check mark rule existance: %w", err)
					}
					if !exists {
						return fmt.Errorf("expected nft mangle input rule does not exist")
					}

					restoreMarkRule := &nftables.Rule{
						Table: table,
						Chain: outputChain,
						Exprs: []expr.Any{
							&expr.Ct{
								Register: 1,
								Key:      expr.CtKeyMARK,
							},
							&expr.Cmp{
								Op:       expr.CmpOpEq,
								Register: 1,
								Data:     binaryutil.NativeEndian.PutUint32(uint32(link.Attrs().Index)),
							},
							&expr.Counter{},
							&expr.Ct{
								Register: 1,
								Key:      expr.CtKeyMARK,
							},
							&expr.Meta{
								Key:            expr.MetaKeyMARK,
								SourceRegister: true,
								Register:       1,
							},
						},
					}

					exists, err = checkNftRule(nft, restoreMarkRule)
					if err != nil {
						return fmt.Errorf("failed to check restore mark rule existance: %w", err)
					}
					if !exists {
						return fmt.Errorf("expected nft mangle output rule does not exist")
					}

				}
			} else {
				return fmt.Errorf("routes are different in table %d and the default table", egressLinkTable)
			}
		} else {
			if len(newRoutes) > 0 {
				return fmt.Errorf("irrelevant routes in table %d", egressLinkTable)
			}
		}
	}

	return nil
}
