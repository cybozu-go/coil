//go:build privileged

package test

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/founat"
	"github.com/cybozu-go/coil/v2/pkg/nat/netfilter"
)

// Network configuration for test topology:
// client eth0 <-> eth0 egress eth1 <-> eth0 target
//
// Subnet allocation:
// - client-egress link: 10.1.1.0/24, fd01::100/120
// - egress-target link: 10.1.3.0/24, fd01::300/120
var (
	// Client namespace addresses
	clientIPv4 = net.ParseIP("10.1.1.2")
	clientIPv6 = net.ParseIP("fd01::102")

	// Egress namespace addresses (eth0 - facing client)
	egressEth0IPv4 = net.ParseIP("10.1.1.1")
	egressEth0IPv6 = net.ParseIP("fd01::101")

	// Egress namespace addresses (eth1 - facing target)
	egressEth1IPv4 = net.ParseIP("10.1.3.2")
	egressEth1IPv6 = net.ParseIP("fd01::302")

	// Target namespace addresses
	targetIPv4 = net.ParseIP("10.1.3.1")
	targetIPv6 = net.ParseIP("fd01::301")

	// Destination networks for NAT routing
	targetNetworkIPv4 = &net.IPNet{IP: net.ParseIP("10.1.3.0"), Mask: net.CIDRMask(24, 32)}
	targetNetworkIPv6 = &net.IPNet{IP: net.ParseIP("fd01::300"), Mask: net.CIDRMask(120, 128)}

	// FoU tunnel port
	fouPort = 5555
)

var _ = Describe("NAT", func() {
	var (
		clientNS ns.NetNS
		egressNS ns.NetNS
		targetNS ns.NetNS
	)

	for _, backend := range []string{constants.EgressBackendIPTables, constants.EgressBackendNFTables} {
		Context(fmt.Sprintf("with %s backend", backend), func() {
			BeforeEach(func() {
				var err error

				clientNS, err = testutils.NewNS()
				Expect(err).NotTo(HaveOccurred())

				egressNS, err = testutils.NewNS()
				Expect(err).NotTo(HaveOccurred())

				targetNS, err = testutils.NewNS()
				Expect(err).NotTo(HaveOccurred())

				setupNetwork(clientNS, egressNS, targetNS)
			})

			AfterEach(func() {
				if clientNS != nil {
					_ = clientNS.Close()
				}
				if egressNS != nil {
					_ = egressNS.Close()
				}
				if targetNS != nil {
					_ = targetNS.Close()
				}
			})

			It("should NAT traffic with originatingOnly=false", func() {
				testNAT(clientNS, egressNS, targetNS, backend, false)
			})

			It("should NAT traffic with originatingOnly=true", func() {
				testNAT(clientNS, egressNS, targetNS, backend, true)
			})
		})
	}
})

func testNAT(clientNS, egressNS, targetNS ns.NetNS, backend string, originatingOnly bool) {
	// Setup client namespace
	err := clientNS.Do(func(ns.NetNS) error {
		ft := founat.NewFoUTunnel(fouPort, clientIPv4, clientIPv6, nil)
		if err := ft.Init(); err != nil {
			return fmt.Errorf("ft.Init on client failed: %w", err)
		}

		nc := founat.NewNatClient(clientIPv4, clientIPv6, nil, backend, nil)
		if err := nc.Init(); err != nil {
			return fmt.Errorf("nc.Init failed: %w", err)
		}

		link4, err := ft.AddPeer(egressEth0IPv4, true)
		if err != nil {
			return fmt.Errorf("ft.AddPeer failed for %s: %w", egressEth0IPv4, err)
		}
		link6, err := ft.AddPeer(egressEth0IPv6, true)
		if err != nil {
			return fmt.Errorf("ft.AddPeer failed for %s: %w", egressEth0IPv6, err)
		}

		if err := nc.AddEgress(link4, []*net.IPNet{targetNetworkIPv4}, originatingOnly); err != nil {
			return fmt.Errorf("nc.AddEgress failed for %s: %w", targetNetworkIPv4, err)
		}
		if err := nc.AddEgress(link6, []*net.IPNet{targetNetworkIPv6}, originatingOnly); err != nil {
			return fmt.Errorf("nc.AddEgress failed for %s: %w", targetNetworkIPv6, err)
		}

		return nil
	})
	Expect(err).NotTo(HaveOccurred())

	// Setup egress namespace
	err = egressNS.Do(func(ns.NetNS) error {
		ft := founat.NewFoUTunnel(fouPort, egressEth0IPv4, egressEth0IPv6, nil)
		if err := ft.Init(); err != nil {
			return fmt.Errorf("ft.Init on egress failed: %w", err)
		}

		n, err := netfilter.NewNatServer("eth1", egressEth0IPv4, egressEth0IPv6, backend)
		if err != nil {
			return fmt.Errorf("netfilter.NewNatServer failed: %w", err)
		}

		link4, err := ft.AddPeer(clientIPv4, true)
		if err != nil {
			return fmt.Errorf("ft.AddPeer failed for %s: %w", clientIPv4, err)
		}
		link6, err := ft.AddPeer(clientIPv6, true)
		if err != nil {
			return fmt.Errorf("ft.AddPeer failed for %s: %w", clientIPv6, err)
		}

		if err := n.AddClient(clientIPv4, link4); err != nil {
			return fmt.Errorf("n.AddClient failed for %s: %w", clientIPv4, err)
		}
		if err := n.AddClient(clientIPv6, link6); err != nil {
			return fmt.Errorf("n.AddClient failed for %s: %w", clientIPv6, err)
		}

		return nil
	})
	Expect(err).NotTo(HaveOccurred())

	// Start HTTP server in target namespace.
	// The entire ns.Do call must be wrapped in a goroutine, not just ListenAndServe.
	// If we started a goroutine inside ns.Do, it would run on a different OS thread
	// that is not in the target namespace (ns.Do only switches the calling thread).
	go func() {
		err := targetNS.Do(func(_ ns.NetNS) error {
			s := &http.Server{Addr: ":80"}
			return s.ListenAndServe()
		})
		Expect(err).NotTo(HaveOccurred(), "HTTP server failed")
	}()
	// Wait for the HTTP server to start listening before running connectivity tests.
	time.Sleep(100 * time.Millisecond)

	// Test connectivity from client to target
	err = clientNS.Do(func(ns.NetNS) error {
		targetURLv4 := fmt.Sprintf("http://%s", targetIPv4)
		out, err := exec.Command("curl", "-s", "--connect-timeout", "5", targetURLv4).CombinedOutput()
		if err != nil {
			return fmt.Errorf("curl over FoU IPv4 failed (backend=%s): %s, %w", backend, string(out), err)
		}

		targetURLv6 := fmt.Sprintf("http://[%s]", targetIPv6)
		out, err = exec.Command("curl", "-s", "--connect-timeout", "5", targetURLv6).CombinedOutput()
		if err != nil {
			return fmt.Errorf("curl over FoU IPv6 failed (backend=%s): %s, %w", backend, string(out), err)
		}
		return nil
	})
	Expect(err).NotTo(HaveOccurred())
}

func setupNetwork(clientNS, egressNS, targetNS ns.NetNS) {
	// Create veth pairs and move them to namespaces
	// client eth0 <-> eth0 egress eth1 <-> eth0 target

	// client <-> egress (eth0)
	runCmd("ip", "link", "add", "veth-client", "type", "veth", "peer", "name", "veth-egress0")
	runCmd("ip", "link", "set", "veth-client", "netns", getNetNSPath(clientNS))
	runCmd("ip", "link", "set", "veth-egress0", "netns", getNetNSPath(egressNS))

	// egress (eth1) <-> target
	runCmd("ip", "link", "add", "veth-egress1", "type", "veth", "peer", "name", "veth-target")
	runCmd("ip", "link", "set", "veth-egress1", "netns", getNetNSPath(egressNS))
	runCmd("ip", "link", "set", "veth-target", "netns", getNetNSPath(targetNS))

	// Configure client namespace
	_ = clientNS.Do(func(ns.NetNS) error {
		runCmd("ip", "link", "set", "veth-client", "name", "eth0")
		runCmd("ip", "link", "set", "eth0", "up")
		runCmd("ip", "addr", "add", clientIPv4.String()+"/24", "dev", "eth0")
		runCmd("ip", "-6", "addr", "add", clientIPv6.String()+"/120", "dev", "eth0", "nodad")
		runCmd("ip", "route", "add", "default", "via", egressEth0IPv4.String())
		runCmd("ip", "-6", "route", "add", "default", "via", egressEth0IPv6.String())
		return nil
	})

	// Configure egress namespace
	_ = egressNS.Do(func(ns.NetNS) error {
		runCmd("ip", "link", "set", "veth-egress0", "name", "eth0")
		runCmd("ip", "link", "set", "veth-egress1", "name", "eth1")
		runCmd("ip", "link", "set", "eth0", "up")
		runCmd("ip", "link", "set", "eth1", "up")
		runCmd("ip", "addr", "add", egressEth0IPv4.String()+"/24", "dev", "eth0")
		runCmd("ip", "-6", "addr", "add", egressEth0IPv6.String()+"/120", "dev", "eth0", "nodad")
		runCmd("ip", "addr", "add", egressEth1IPv4.String()+"/24", "dev", "eth1")
		runCmd("ip", "-6", "addr", "add", egressEth1IPv6.String()+"/120", "dev", "eth1", "nodad")
		runCmd("sysctl", "-w", "net.ipv4.ip_forward=1")
		runCmd("sysctl", "-w", "net.ipv6.conf.all.forwarding=1")
		return nil
	})

	// Configure target namespace
	_ = targetNS.Do(func(ns.NetNS) error {
		runCmd("ip", "link", "set", "lo", "up")
		runCmd("ip", "link", "set", "veth-target", "name", "eth0")
		runCmd("ip", "link", "set", "eth0", "up")
		runCmd("ip", "addr", "add", targetIPv4.String()+"/24", "dev", "eth0")
		runCmd("ip", "-6", "addr", "add", targetIPv6.String()+"/120", "dev", "eth0", "nodad")
		runCmd("ip", "route", "add", "default", "via", egressEth1IPv4.String())
		runCmd("ip", "-6", "route", "add", "default", "via", egressEth1IPv6.String())
		return nil
	})
}

func getNetNSPath(netNS ns.NetNS) string {
	return netNS.Path()
}
