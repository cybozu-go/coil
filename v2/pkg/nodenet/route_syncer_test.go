package nodenet

import (
	"errors"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	ctrl "sigs.k8s.io/controller-runtime"
)

func setupFake(t *testing.T) {
	dummy := &netlink.Dummy{}
	dummy.Name = "dummy"
	if err := netlink.LinkAdd(dummy); err != nil {
		t.Fatal(err)
	}
	dummyLink, err := netlink.LinkByName("dummy")
	if err != nil {
		t.Fatal(err)
	}
	if err := netlink.LinkSetUp(dummyLink); err != nil {
		t.Fatal(err)
	}

	// fake router addresses
	if err := netlink.AddrAdd(dummyLink, &netlink.Addr{
		IPNet: &net.IPNet{IP: net.ParseIP("10.9.0.1"), Mask: net.CIDRMask(24, 32)},
		Scope: unix.RT_SCOPE_LINK,
	}); err != nil {
		t.Fatal(err)
	}
	if err := netlink.AddrAdd(dummyLink, &netlink.Addr{
		IPNet: &net.IPNet{IP: net.ParseIP("fd09::1"), Mask: net.CIDRMask(120, 128)},
		Scope: unix.RT_SCOPE_LINK,
	}); err != nil {
		t.Fatal(err)
	}
	ip.SettleAddresses("dummy", 10)

	// existing route
	if err := netlink.RouteAdd(&netlink.Route{
		Dst:      netlink.NewIPNet(net.ParseIP("10.10.10.10")),
		Gw:       net.ParseIP("127.0.0.1"),
		Protocol: 2,
	}); err != nil {
		t.Fatal(err)
	}
}

func checkRoutingTable(r RouteSyncer, expected []GatewayInfo) error {
	if err := r.Sync(expected); err != nil {
		return err
	}

	routes, err := netlink.RouteList(nil, 0)
	if err != nil {
		return err
	}

	routeMap := make(map[string]bool)
	var nCoil int
	for _, r := range routes {
		routeMap[r.Gw.String()+" "+r.Dst.String()] = true
		if r.Protocol == 31 {
			nCoil++
		}
	}

	if !routeMap[net.ParseIP("127.0.0.1").String()+" "+netlink.NewIPNet(net.ParseIP("10.10.10.10")).String()] {
		return errors.New("intact route is not found: 10.10.10.10")
	}

	for _, gi := range expected {
		gwStr := gi.Gateway.String()
		for _, n := range gi.Networks {
			if !routeMap[gwStr+" "+n.String()] {
				return fmt.Errorf("expected route %s for %s not found", n.String(), gwStr)
			}
			nCoil--
		}
	}

	if nCoil != 0 {
		return fmt.Errorf("some routes were not deleted: %d", nCoil)
	}
	return nil
}

func TestRouteSyncer(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("need root privilege")
	}

	setupFake(t)

	r := NewRouteSyncer(31, ctrl.Log.WithName("test"))

	gws := []GatewayInfo{
		{net.ParseIP("10.9.0.2"), []*net.IPNet{
			{IP: net.ParseIP("192.168.1.0"), Mask: net.CIDRMask(24, 32)},
			{IP: net.ParseIP("192.168.2.0"), Mask: net.CIDRMask(24, 32)},
		}},
		{net.ParseIP("fd09::2"), []*net.IPNet{
			{IP: net.ParseIP("fd03::0100"), Mask: net.CIDRMask(120, 128)},
			{IP: net.ParseIP("fd03::0200"), Mask: net.CIDRMask(120, 128)},
		}},
	}
	if err := checkRoutingTable(r, gws); err != nil {
		t.Fatal(err)
	}

	gws = []GatewayInfo{
		{net.ParseIP("10.9.0.2"), []*net.IPNet{
			{IP: net.ParseIP("192.168.2.0"), Mask: net.CIDRMask(24, 32)},
		}},
		{net.ParseIP("fd09::2"), []*net.IPNet{
			{IP: net.ParseIP("fd03::0100"), Mask: net.CIDRMask(120, 128)},
			{IP: net.ParseIP("fd03::0200"), Mask: net.CIDRMask(120, 128)},
		}},
	}
	if err := checkRoutingTable(r, gws); err != nil {
		t.Fatal(err)
	}

	gws = []GatewayInfo{
		{net.ParseIP("10.9.0.2"), []*net.IPNet{
			{IP: net.ParseIP("192.168.2.0"), Mask: net.CIDRMask(24, 32)},
		}},
		{net.ParseIP("10.9.0.3"), []*net.IPNet{
			{IP: net.ParseIP("192.168.1.0"), Mask: net.CIDRMask(24, 32)},
		}},
		{net.ParseIP("fd09::2"), []*net.IPNet{
			{IP: net.ParseIP("fd03::0100"), Mask: net.CIDRMask(120, 128)},
		}},
	}
	if err := checkRoutingTable(r, gws); err != nil {
		t.Fatal(err)
	}
}
