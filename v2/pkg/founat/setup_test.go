package founat

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getuid() != 0 {
		os.Exit(0)
	}

	setup()
	os.Exit(m.Run())
}

const (
	nsClient = "nat-client"
	nsRouter = "nat-router"
	nsEgress = "nat-egress"
	nsTarget = "nat-target"
)

func runIP(args ...string) {
	out, err := exec.Command("ip", args...).CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("%s: %v", string(out), err))
	}
}

func netnsExec(nsName string, args ...string) {
	nargs := append([]string{"netns", "exec", nsName}, args...)
	runIP(nargs...)
}

func setup() {
	// setup network as follows:
	// client eth0 <-> eth0 router eth1 <-> eth0 egress eth1 <-> eth0 target
	runIP("link", "add", "veth-coil", "type", "veth", "peer", "name", "veth-coil2")
	runIP("link", "set", "veth-coil", "netns", nsClient, "name", "eth0", "up")
	runIP("link", "set", "veth-coil2", "netns", nsRouter, "name", "eth0", "up")
	runIP("link", "add", "veth-coil", "type", "veth", "peer", "name", "veth-coil2")
	runIP("link", "set", "veth-coil", "netns", nsRouter, "name", "eth1", "up")
	runIP("link", "set", "veth-coil2", "netns", nsEgress, "name", "eth0", "up")
	runIP("link", "add", "veth-coil", "type", "veth", "peer", "name", "veth-coil2")
	runIP("link", "set", "veth-coil", "netns", nsEgress, "name", "eth1", "up")
	runIP("link", "set", "veth-coil2", "netns", nsTarget, "name", "eth0", "up")

	// assign IP addresses
	// 10.1.1.0/24,fd01::100/120 for client-router
	// 10.1.2.0/24,fd01::200/120 for router-egress
	// 10.1.3.0/24,fd01::300/120 for egress-target
	netnsExec(nsClient, "ip", "a", "add", "10.1.1.2/24", "dev", "eth0")
	netnsExec(nsClient, "ip", "a", "add", "fd01::102/120", "dev", "eth0", "nodad")
	netnsExec(nsRouter, "ip", "a", "add", "10.1.1.1/24", "dev", "eth0")
	netnsExec(nsRouter, "ip", "a", "add", "fd01::101/120", "dev", "eth0", "nodad")
	netnsExec(nsRouter, "ip", "a", "add", "10.1.2.1/24", "dev", "eth1")
	netnsExec(nsRouter, "ip", "a", "add", "fd01::201/120", "dev", "eth1", "nodad")
	netnsExec(nsEgress, "ip", "a", "add", "10.1.2.2/24", "dev", "eth0")
	netnsExec(nsEgress, "ip", "a", "add", "fd01::202/120", "dev", "eth0", "nodad")
	netnsExec(nsEgress, "ip", "a", "add", "10.1.3.2/24", "dev", "eth1")
	netnsExec(nsEgress, "ip", "a", "add", "fd01::302/120", "dev", "eth1", "nodad")
	netnsExec(nsTarget, "ip", "a", "add", "10.1.3.1/24", "dev", "eth0")
	netnsExec(nsTarget, "ip", "a", "add", "fd01::301/120", "dev", "eth0", "nodad")

	// setup routing
	netnsExec(nsRouter, "sysctl", "-w", "net.ipv4.ip_forward=1")
	netnsExec(nsRouter, "sysctl", "-w", "net.ipv6.conf.all.forwarding=1")
	netnsExec(nsClient, "ip", "route", "add", "default", "via", "10.1.1.1")
	netnsExec(nsClient, "ip", "-6", "route", "add", "default", "via", "fd01::101")
	netnsExec(nsEgress, "ip", "route", "add", "default", "via", "10.1.2.1")
	netnsExec(nsEgress, "ip", "-6", "route", "add", "default", "via", "fd01::201")
	netnsExec(nsClient, "ping", "-4", "-c", "1", "10.1.2.2")
	netnsExec(nsClient, "ping", "-6", "-c", "1", "fd01::202")
}
