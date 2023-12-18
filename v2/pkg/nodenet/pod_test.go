package nodenet

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"
)

func nsPath(pod string) string {
	return filepath.Join("/run/netns", pod)
}

func TestPodNetwork(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("run as root")
	}

	pn := NewPodNetwork(116, 2000, 30, net.ParseIP("10.20.30.41"), net.ParseIP("fd10::41"),
		false, false, ctrl.Log.WithName("pod-network"))
	if err := pn.Init(); err != nil {
		t.Fatal(err)
	}

	podConf1 := &PodNetConf{
		PoolName:    "default",
		ContainerId: "c8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a",
		IFace:       "eth0",
		IPv4:        net.ParseIP("10.1.2.3"),
		IPv6:        net.ParseIP("fd02::1"),
	}
	var givenIPv4, givenIPv6 net.IP
	result, err := pn.Setup(nsPath("pod1"), "pod1", "ns1", podConf1, func(ipv4, ipv6 net.IP) error {
		givenIPv4 = ipv4
		givenIPv6 = ipv6
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if !givenIPv4.Equal(net.ParseIP("10.1.2.3")) {
		t.Error("hook could not catch IPv4", givenIPv4)
	}
	if !givenIPv6.Equal(net.ParseIP("fd02::1")) {
		t.Error("hook could not catch IPv6", givenIPv6)
	}

	if len(result.Interfaces) != 2 {
		t.Error(`len(result.Interfaces) != 2`)
	} else {
		cIface := result.Interfaces[0]
		if cIface.Name != "eth0" {
			t.Error(`cIface.Name != "eth0"`)
		}
		if cIface.Sandbox != nsPath("pod1") {
			t.Error(`cIface.Sandbox != nsPath("pod1")`)
		}
	}
	if len(result.IPs) != 2 {
		t.Error(`len(result.IPs) != 2`)
	} else {
		if !result.IPs[0].Address.IP.Equal(net.ParseIP("10.1.2.3")) {
			t.Error(`!result.IPs[0].Address.IP.Equal(net.ParseIP("10.1.2.3"))`)
		}
		if result.IPs[0].Address.IP.To4() == nil {
			t.Error(`!result.IPs[0] version != "4"`)
		}
		if !result.IPs[1].Address.IP.Equal(net.ParseIP("fd02::1")) {
			t.Error(`!result.IPs[1].Address.IP.Equal(net.ParseIP("fd02::1"))`)
		}
		if result.IPs[1].Address.IP.To4() != nil {
			t.Error(`!result.IPs[1] version != "6"`)
		}
	}
	if result.CNIVersion != "1.0.0" {
		t.Error(`CNI version != 1.0.0`)
	}

	// run a test HTTP server
	go func() {
		serv := &http.Server{
			Addr:    ":8000",
			Handler: http.NotFoundHandler(),
		}
		serv.ListenAndServe()
	}()

	err = exec.Command("ip", "link", "add", "foo", "type", "dummy").Run()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		exec.Command("ip", "link", "del", "foo").Run()
	}()

	err = exec.Command("ip", "link", "set", "foo", "up").Run()
	if err != nil {
		t.Fatal(err)
	}
	err = exec.Command("ip", "addr", "add", "10.100.0.1/32", "dev", "foo").Run()
	if err != nil {
		t.Fatal(err)
	}
	err = exec.Command("ip", "addr", "add", "fd02::100/128", "dev", "foo").Run()
	if err != nil {
		t.Fatal(err)
	}

	// test routing between pod <-> host
	err = exec.Command("ip", "netns", "exec", "pod1", "curl", "-s", "http://10.100.0.1:8000").Run()
	if err != nil {
		t.Error("curl to host over IPv4 failed")
	}
	err = exec.Command("ip", "netns", "exec", "pod1", "curl", "-s", "http://[fd02::100]:8000").Run()
	if err != nil {
		t.Error("curl to host over IPv6 failed")
	}

	err = pn.Update(podConf1.IPv4, podConf1.IPv6, func(ipv4, ipv6 net.IP) error {
		givenIPv4 = ipv4
		givenIPv6 = ipv6
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !givenIPv4.Equal(net.ParseIP("10.1.2.3")) {
		t.Error("hook could not catch IPv4", givenIPv4)
	}
	if !givenIPv6.Equal(net.ParseIP("fd02::1")) {
		t.Error("hook could not catch IPv6", givenIPv6)
	}

	err = pn.Check(podConf1.ContainerId, podConf1.IFace)
	if err != nil {
		t.Error(err)
	}

	confs, err := pn.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(confs) != 1 {
		t.Fatal(`len(confs) != 1`)
	}
	if confs[0].ContainerId != podConf1.ContainerId {
		t.Error(`confs[0].ContainerId != podConf1.ContainerId`)
	}
	if confs[0].IFace != podConf1.IFace {
		t.Error(`confs[0].IFace != podConf1.IFace`)
	}
	if confs[0].PoolName != podConf1.PoolName {
		t.Error(`confs[0].PoolName != podConf1.PoolName`)
	}
	if !confs[0].IPv4.Equal(podConf1.IPv4) {
		t.Error(`!confs[0].IPv4.Equal(podConf1.IPv4)`)
	}
	if !confs[0].IPv6.Equal(podConf1.IPv6) {
		t.Error(`!confs[0].IPv6.Equal(podConf1.IPv6)`, confs[0].IPv6)
	}

	// create IPv4 only pod

	podConf2 := &PodNetConf{
		PoolName:    "default",
		ContainerId: "d8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a",
		IFace:       "eth0",
		IPv4:        net.ParseIP("10.1.2.4"),
	}
	result, err = pn.Setup(nsPath("pod2"), "pod2", "ns1", podConf2, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.IPs) != 1 {
		t.Fatal(`len(result.IPs) != 1`)
	}
	if !result.IPs[0].Address.IP.Equal(net.ParseIP("10.1.2.4")) {
		t.Error(`!result.IPs[0].Address.IP.Equal(net.ParseIP("10.1.2.4"))`)
	}

	// test routing between pods over IPv4
	err = exec.Command("ip", "netns", "exec", "pod2", "ping", "-c", "3", "-i", "0.2", "10.1.2.3").Run()
	if err != nil {
		t.Error("ping to pod1 over IPv4 failed")
	}

	err = pn.Update(podConf2.IPv4, podConf2.IPv6, func(ipv4, ipv6 net.IP) error {
		givenIPv4 = ipv4
		givenIPv6 = ipv6
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = pn.Check(podConf2.ContainerId, podConf2.IFace)
	if err != nil {
		t.Error(err)
	}

	confs, err = pn.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(confs) != 2 {
		t.Fatal(`len(confs) != 2`)
	}
	found := false
	for _, c := range confs {
		if c.ContainerId == "d8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a" {
			found = true
			if !c.IPv4.Equal(podConf2.IPv4) {
				t.Error(`!c.IPv4.Equal(podConf2.IPv4)`)
			}
			if c.IPv6 != nil {
				t.Error(`c.IPv6 != nil`)
			}
		}
	}
	if !found {
		t.Error("config for pod2 not found")
	}

	// create IPv6 only pod

	podConf3 := &PodNetConf{
		PoolName:    "default",
		ContainerId: "00f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a",
		IFace:       "eth0",
		IPv6:        net.ParseIP("fd02::3"),
	}
	result, err = pn.Setup(nsPath("pod3"), "pod3", "ns1", podConf3, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.IPs) != 1 {
		t.Fatal(`len(result.IPs) != 1`)
	}
	if !result.IPs[0].Address.IP.Equal(net.ParseIP("fd02::3")) {
		t.Error(`!result.IPs[0].Address.IP.Equal(net.ParseIP("fd02::3"))`)
	}

	// test routing between pods over IPv6
	err = exec.Command("ip", "netns", "exec", "pod3", "ping", "-c", "3", "-i", "0.2", "fd02::1").Run()
	if err != nil {
		t.Error("ping to pod1 over IPv6 failed")
	}

	err = pn.Update(podConf3.IPv4, podConf3.IPv6, func(ipv4, ipv6 net.IP) error {
		givenIPv4 = ipv4
		givenIPv6 = ipv6
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = pn.Check(podConf3.ContainerId, podConf3.IFace)
	if err != nil {
		t.Error(err)
	}

	confs, err = pn.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(confs) != 3 {
		t.Fatal(`len(confs) != 3`)
	}
	found = false
	for _, c := range confs {
		if c.ContainerId == "00f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a" {
			found = true
			if !c.IPv6.Equal(podConf3.IPv6) {
				t.Error(`!c.IPv6.Equal(podConf2.IPv6)`)
			}
			if c.IPv4 != nil {
				t.Error(`c.IPv4 != nil`)
			}
		}
	}
	if !found {
		t.Error("config for pod3 not found")
	}

	// This test is for https://github.com/cybozu-go/coil/pull/265.
	// Confirm to select the expected pod.
	// In this test, the address in podConf is equal to the address in the hook function.
	// The hook function is executed inside the pod's network namespace.
	type addrInfo struct {
		Family string `json:"family"`
		Local  string `json:"local"`
	}
	type devInfo struct {
		IfName    string     `json:"ifname"`
		AddrInfos []addrInfo `json:"addr_info"`
	}
	info := make([]devInfo, 0, 1)
	err = pn.Update(podConf1.IPv4, nil, func(ipv4, ipv6 net.IP) error {
		out, err := exec.Command("ip", "-j", "addr", "show", podConf1.IFace).Output()
		if err != nil {
			return err
		}
		if err := json.Unmarshal(out, &info); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	if len(info) != 1 {
		t.Fatal("len(info) != 1")
	}

	if info[0].IfName != podConf1.IFace {
		t.Fatalf("expected iface is %s", podConf1.IFace)
	}

	for _, addr := range info[0].AddrInfos {
		if addr.Family != "inet" {
			continue
		}
		if addr.Local != podConf1.IPv4.String() {
			t.Fatal("address don't match")
		}
	}

	// destroy pod2 network
	err = pn.Destroy(podConf2.ContainerId, podConf2.IFace)
	if err != nil {
		t.Error(err)
	}

	confs, err = pn.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(confs) != 2 {
		t.Fatal(`len(confs) != 2`)
	}
	for _, c := range confs {
		if c.ContainerId == "d8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a" {
			t.Error(`pod network 2 has not been destroyed`)
		}
	}

	// destroy should be idempotent
	err = pn.Destroy(podConf2.ContainerId, podConf2.IFace)
	if err != nil {
		t.Error(err)
	}
}
