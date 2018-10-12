package coild

import (
	"net"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"testing"
)

func routeExists(r *net.IPNet, tableID string) bool {
	out, err := exec.Command("ip", "-o", "route", "show", "table", tableID).Output()
	if err != nil {
		panic(err)
	}

	lines := strings.Split(string(out), "\n")
	tt := r.IP.String()

	for _, line := range lines {
		if strings.HasPrefix(line, tt) {
			return true
		}
	}
	return false
}

func testRoutingAdd(t *testing.T) {
	t.Parallel()

	err := exec.Command("ip", "route", "flush", "table", "118").Run()
	if err != nil {
		t.Fatal(err)
	}

	_, block, _ := net.ParseCIDR("192.168.100.1/32")
	err = addBlockRouting(118, 30, block)
	if err != nil {
		t.Fatal(err)
	}

	if !routeExists(block, "118") {
		t.Error("route not found:", block)
	}
}

func testRoutingSync(t *testing.T) {
	t.Parallel()

	err := exec.Command("ip", "route", "flush", "table", "119").Run()
	if err != nil {
		t.Fatal(err)
	}

	_, block1, _ := net.ParseCIDR("192.168.100.1/32")
	_, block2, _ := net.ParseCIDR("192.168.100.2/32")
	_, block3, _ := net.ParseCIDR("192.168.100.3/32")

	err = syncRoutingTable(119, 30, []*net.IPNet{block1, block2})
	if err != nil {
		t.Fatal(err)
	}

	if !routeExists(block1, "119") {
		t.Error("route not found:", block1)
	}
	if !routeExists(block2, "119") {
		t.Error("route not found:", block2)
	}

	err = syncRoutingTable(119, 30, []*net.IPNet{block1, block3})
	if err != nil {
		t.Fatal(err)
	}

	if !routeExists(block1, "119") {
		t.Error("route not found:", block1)
	}
	if routeExists(block2, "119") {
		t.Error("route not removed:", block2)
	}
	if !routeExists(block3, "119") {
		t.Error("route not found:", block3)
	}
}

func TestRouting(t *testing.T) {
	if os.Getenv("CIRCLECI") == "true" {
		t.Skip("CircleCI does not allow editing routes")
	}

	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if u.Uid != "0" {
		t.Skip("only for root user")
	}
	t.Run("Add", testRoutingAdd)
	t.Run("Sync", testRoutingSync)
}
