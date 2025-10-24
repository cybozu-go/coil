package founat

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"

	"github.com/cybozu-go/coil/v2/pkg/constants"
)

func TestNAT(t *testing.T) {
	t.Parallel()

	for _, backend := range []string{constants.EgressBackendIPTables, constants.EgressBackendNFTables} {
		backend := backend

		t.Logf("Testing with backend=%s", backend)

		cNS := getNS(nsClient)
		defer cNS.Close()
		eNS := getNS(nsEgress)
		defer eNS.Close()
		targetNS := getNS(nsTarget)
		defer targetNS.Close()

		if err := testNatFunc(t, cNS, eNS, targetNS, backend, false); err != nil {
			t.Fatal(err)
		}

		if err := testNatFunc(t, cNS, eNS, targetNS, backend, true); err != nil {
			t.Fatal(err)
		}
	}
}

func testNatFunc(t *testing.T, cNS, eNS, targetNS ns.NetNS, backend string, originatingOnly bool) error {
	err := cNS.Do(func(ns.NetNS) error {
		ft := NewFoUTunnel(5555, net.ParseIP("10.1.1.2"), net.ParseIP("fd01::102"), nil)
		if err := ft.Init(); err != nil {
			return fmt.Errorf("ft.Init on client failed: %w", err)
		}

		nc := NewNatClient(net.ParseIP("10.1.1.2"), net.ParseIP("fd01::102"), nil, nil, backend)
		if err := nc.Init(); err != nil {
			return fmt.Errorf("nc.Init failed: %w", err)
		}

		link4, err := ft.AddPeer(net.ParseIP("10.1.2.2"), true)
		if err != nil {
			return fmt.Errorf("ft.AddPeer failed for 10.1.2.2: %w", err)
		}
		link6, err := ft.AddPeer(net.ParseIP("fd01::202"), true)
		if err != nil {
			return fmt.Errorf("ft.AddPeer failed for fd01::202: %w", err)
		}

		err = nc.AddEgress(link4, []*net.IPNet{{IP: net.ParseIP("10.1.3.0"), Mask: net.CIDRMask(24, 32)}}, originatingOnly)
		if err != nil {
			return fmt.Errorf("nc.AddEgress failed for 10.1.3.0/24: %w", err)
		}
		err = nc.AddEgress(link6, []*net.IPNet{{IP: net.ParseIP("fd01::300"), Mask: net.CIDRMask(120, 128)}}, originatingOnly)
		if err != nil {
			return fmt.Errorf("nc.AddEgress failed for fd01::300/120: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	err = eNS.Do(func(ns.NetNS) error {
		ft := NewFoUTunnel(5555, net.ParseIP("10.1.2.2"), net.ParseIP("fd01::202"), nil)
		if err := ft.Init(); err != nil {
			return fmt.Errorf("ft.Init on egress failed: %w", err)
		}

		egress := NewEgress("eth1", net.ParseIP("10.1.2.2"), net.ParseIP("fd01::202"), backend)
		if err := egress.Init(); err != nil {
			return fmt.Errorf("egress.Init failed: %w", err)
		}

		link4, err := ft.AddPeer(net.ParseIP("10.1.1.2"), true)
		if err != nil {
			return fmt.Errorf("ft.AddPeer failed for 10.1.1.2: %w", err)
		}
		link6, err := ft.AddPeer(net.ParseIP("fd01::102"), true)
		if err != nil {
			return fmt.Errorf("ft.AddPeer failed for fd01::102: %w", err)
		}

		if err := egress.AddClient(net.ParseIP("10.1.1.2"), link4); err != nil {
			return fmt.Errorf("egress.AddClient failed for 10.1.1.2: %w", err)
		}
		if err := egress.AddClient(net.ParseIP("fd01::102"), link6); err != nil {
			return fmt.Errorf("egress.AddClient failed for 10.1.1.2: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	go targetNS.Do(func(_ ns.NetNS) error {
		s := &http.Server{}
		t.Logf("httpd running in the target network namespace (backend=%s)", backend)
		s.ListenAndServe()
		return nil
	})
	time.Sleep(100 * time.Millisecond)

	err = cNS.Do(func(ns.NetNS) error {
		out, err := exec.Command("curl", "http://10.1.3.1").CombinedOutput()
		if err != nil {
			return fmt.Errorf("curl over fou IPv4 failed (backend=%s): %s, %w", backend, string(out), err)
		}
		out, err = exec.Command("curl", "http://[fd01::301]").CombinedOutput()
		if err != nil {
			return fmt.Errorf("curl over fou IPv6 failed (backend=%s): %s, %w", backend, string(out), err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
