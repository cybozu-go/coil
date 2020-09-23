package nodenet

import (
	"net"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vishvananda/netlink"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	testTable    = 133
	testProtocol = 99
)

func getRoutes(t *testing.T) map[string]bool {
	h, err := netlink.NewHandle()
	if err != nil {
		t.Fatal(err)
	}
	defer h.Delete()

	filter := &netlink.Route{Table: testTable}
	routes, err := h.RouteListFiltered(0, filter, netlink.RT_FILTER_TABLE)
	if err != nil {
		t.Fatal(err)
	}

	result := make(map[string]bool)
	for _, route := range routes {
		if route.Dst == nil {
			continue
		}
		result[route.Dst.String()] = true
	}
	return result
}

func TestRouteExporter(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("need root privilege")
	}

	_, n1, _ := net.ParseCIDR("10.2.0.0/27")
	_, n2, _ := net.ParseCIDR("10.3.0.0/31")
	_, n3, _ := net.ParseCIDR("fd02::0200/123")
	_, n4, _ := net.ParseCIDR("fd02::0300/127")

	exporter := NewRouteExporter(testTable, testProtocol, ctrl.Log.WithName("exporter"))
	err := exporter.Sync([]*net.IPNet{n1, n2, n3, n4})
	if err != nil {
		t.Fatal(err)
	}

	routes := getRoutes(t)
	if !cmp.Equal(routes, map[string]bool{
		"10.2.0.0/27":   true,
		"10.3.0.0/31":   true,
		"fd02::200/123": true,
		"fd02::300/127": true,
	}) {
		t.Error("mismatch1", routes)
	}

	err = exporter.Sync([]*net.IPNet{n1, n3})
	if err != nil {
		t.Fatal(err)
	}

	routes = getRoutes(t)
	if !cmp.Equal(routes, map[string]bool{
		"10.2.0.0/27":   true,
		"fd02::200/123": true,
	}) {
		t.Error("mismatch2", routes)
	}

	err = exporter.Sync([]*net.IPNet{n1, n2, n4})
	if err != nil {
		t.Fatal(err)
	}

	routes = getRoutes(t)
	if !cmp.Equal(routes, map[string]bool{
		"10.2.0.0/27":   true,
		"10.3.0.0/31":   true,
		"fd02::300/127": true,
	}) {
		t.Error("mismatch3", routes)
	}

	err = exporter.Sync(nil)
	if err != nil {
		t.Fatal(err)
	}

	routes = getRoutes(t)
	if len(routes) != 0 {
		t.Error("could not clear routing table")
	}
}
