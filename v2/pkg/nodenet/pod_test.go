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

var podConfMap = map[string]PodNetConf{
	"pod1": {
		PoolName:    "default",
		ContainerId: "c8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a",
		IFace:       "eth0",
		IPv4:        net.ParseIP("10.1.2.3"),
		IPv6:        net.ParseIP("fd02::1"),
	},
	"pod2": {
		PoolName:    "default",
		ContainerId: "d8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a",
		IFace:       "eth0",
		IPv4:        net.ParseIP("10.1.2.4"),
	},
	"pod3": {
		PoolName:    "default",
		ContainerId: "00f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a",
		IFace:       "eth0",
		IPv6:        net.ParseIP("fd02::3"),
	},
	"pod4": {
		PoolName:    "default",
		ContainerId: "3290748f91c8044b6c9b754e5eaa8f3190a2d2915ee371e1dc866b78aa4764ae",
		IFace:       "eth0",
		IPv4:        net.ParseIP("10.1.2.5"),
		IPv6:        net.ParseIP("fd02::4"),
	},
	"pod5": {
		PoolName:    "default",
		ContainerId: "368df12902d559b568aab1d4642943c2c5322bdd17457cdec8081a988e1a2ddf",
		IFace:       "eth0",
		IPv4:        net.ParseIP("10.1.2.6"),
	},
	"pod6": {
		PoolName:    "default",
		ContainerId: "c80bcafb191e73ba0c269609ac6992e362ff3f042f66dfb82894b577673586f4",
		IFace:       "eth0",
		IPv6:        net.ParseIP("fd02::5"),
	},
}

func TestPodNetwork(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("run as root")
	}

	pn := NewPodNetwork(116, 2000, 30, net.ParseIP("10.20.30.41"), net.ParseIP("fd10::41"),
		false, false, ctrl.Log.WithName("pod-network"), true)
	if err := pn.Init(); err != nil {
		t.Fatal(err)
	}

	var givenIPv4, givenIPv6 net.IP

	for name, conf := range podConfMap {

		result, err := pn.SetupIPAM(nsPath(name), name, "ns1", &conf)
		if err != nil {
			t.Fatal(err)
		}
		err = pn.SetupEgress(nsPath(name), &conf, func(ipv4, ipv6 net.IP) error {
			givenIPv4 = ipv4
			givenIPv6 = ipv6
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if !givenIPv4.Equal(conf.IPv4) {
			t.Error("hook could not catch IPv4", givenIPv4)
		}
		if !givenIPv6.Equal(conf.IPv6) {
			t.Error("hook could not catch IPv6", givenIPv6)
		}

		cIface := result.Interfaces[0]
		if cIface.Name != conf.IFace {
			t.Errorf(`cIface.Name != "%s"`, conf.IFace)
		}
		if cIface.Sandbox != nsPath(name) {
			t.Errorf(`cIface.Sandbox != nsPath("%s")`, name)
		}
		if isDualStack(&conf) {
			if len(result.Interfaces) != 2 {
				t.Error(`len(result.Interfaces) != 2`)
			}
			if len(result.IPs) != 2 {
				t.Error(`len(result.IPs) != 2`)
			}
			if !result.IPs[0].Address.IP.Equal(conf.IPv4) {
				t.Errorf(`!result.IPs[0].Address.IP.Equal("%s")`, conf.IPv4)
			}
			if result.IPs[0].Address.IP.To4() == nil {
				t.Error(`!result.IPs[0] version != "4"`)
			}
			if !result.IPs[1].Address.IP.Equal(conf.IPv6) {
				t.Errorf(`result.IPs[1].Address.IP.Equal("%s")`, conf.IPv6)
			}
			if result.IPs[1].Address.IP.To4() != nil {
				t.Error(`result.IPs[1] version != "6"`)
			}
		} else {
			if len(result.Interfaces) != 2 {
				t.Error(`len(result.Interfaces) != 2`)
			}
			if len(result.IPs) != 1 {
				t.Error(`len(result.IPs) != 1`)
			}
			if conf.IPv4 != nil {
				if !result.IPs[0].Address.IP.Equal(conf.IPv4) {
					t.Errorf(`result.IPs[0].Address.IP.Equal("%s")`, conf.IPv4)
				}
				if result.IPs[0].Address.IP.To4() == nil {
					t.Error(`result.IPs[0] version != "4"`)
				}
			} else {
				if !result.IPs[0].Address.IP.Equal(conf.IPv6) {
					t.Errorf(`!result.IPs[0].Address.IP.Equal("%s")`, conf.IPv6)
				}
				if result.IPs[0].Address.IP.To4() != nil {
					t.Error(`result.IPs[1] version != "6"`)
				}
			}
		}
		if result.CNIVersion != "1.1.0" {
			t.Errorf(`CNI version != 1.1.0 but %s`, result.CNIVersion)
		}
	}

	// run a test HTTP server
	go func() {
		serv := &http.Server{
			Addr:    ":8000",
			Handler: http.NotFoundHandler(),
		}
		serv.ListenAndServe()
	}()

	err := exec.Command("ip", "link", "add", "foo", "type", "dummy").Run()
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

	// test routing between pod2 over IPv4
	err = exec.Command("ip", "netns", "exec", "pod2", "ping", "-c", "3", "-i", "0.2", "10.1.2.3").Run()
	if err != nil {
		t.Error("ping to pod1 over IPv4 failed")
	}

	// test routing between pod3 over IPv6
	err = exec.Command("ip", "netns", "exec", "pod3", "ping", "-c", "3", "-i", "0.2", "fd02::1").Run()
	if err != nil {
		t.Error("ping to pod1 over IPv6 failed")
	}

	// test routing between pod5 over IPv4
	err = exec.Command("ip", "netns", "exec", "pod5", "ping", "-c", "3", "-i", "0.2", "10.1.2.3").Run()
	if err != nil {
		t.Error("ping to pod1 over IPv4 failed")
	}

	// test routing between pod6 over IPv6
	err = exec.Command("ip", "netns", "exec", "pod6", "ping", "-c", "3", "-i", "0.2", "fd02::1").Run()
	if err != nil {
		t.Error("ping to pod1 over IPv6 failed")
	}

	// update pod1
	err = pn.Update(podConfMap["pod1"].IPv4, podConfMap["pod1"].IPv6, func(ipv4, ipv6 net.IP) error {
		givenIPv4 = ipv4
		givenIPv6 = ipv6
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !givenIPv4.Equal(net.ParseIP("10.1.2.3")) {
		t.Error("hook could not catch IPv4", givenIPv4)
	}
	if !givenIPv6.Equal(net.ParseIP("fd02::1")) {
		t.Error("hook could not catch IPv6", givenIPv6)
	}

	err = pn.Check(podConfMap["pod1"].ContainerId, podConfMap["pod1"].IFace)
	if err != nil {
		t.Error(err)
	}

	confs, err := pn.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, conf := range confs {
		if conf.ContainerId == podConfMap["pod1"].ContainerId {
			found = true
			if conf.ContainerId != podConfMap["pod1"].ContainerId {
				t.Errorf(`confs[%d].ContainerId != podConf1.ContainerId`, i)
			}
			if conf.IFace != podConfMap["pod1"].IFace {
				t.Errorf(`confs[%d].IFace != podConf1.IFace`, i)
			}
			if conf.PoolName != podConfMap["pod1"].PoolName {
				t.Errorf(`confs[%d].PoolName != podConf1.PoolName`, i)
			}
			if !conf.IPv4.Equal(podConfMap["pod1"].IPv4) {
				t.Errorf(`!confs[%d].IPv4.Equal(podConf1.IPv4)`, i)
			}
			if !conf.IPv6.Equal(podConfMap["pod1"].IPv6) {
				t.Errorf(`!confs[%d].IPv6.Equal(podConf1.IPv6) %s`, i, confs[0].IPv6)
			}
			break
		}
	}
	if !found {
		t.Error("config for pod1 not found")
	}

	// update pod2
	err = pn.Update(podConfMap["pod2"].IPv4, podConfMap["pod2"].IPv6, func(ipv4, ipv6 net.IP) error {
		givenIPv4 = ipv4
		givenIPv6 = ipv6
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = pn.Check(podConfMap["pod2"].ContainerId, podConfMap["pod2"].IFace)
	if err != nil {
		t.Error(err)
	}

	confs, err = pn.List()
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, c := range confs {
		if c.ContainerId == "d8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a" {
			found = true
			if !c.IPv4.Equal(podConfMap["pod2"].IPv4) {
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

	err = pn.Update(podConfMap["pod3"].IPv4, podConfMap["pod3"].IPv6, func(ipv4, ipv6 net.IP) error {
		givenIPv4 = ipv4
		givenIPv6 = ipv6
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = pn.Check(podConfMap["pod3"].ContainerId, podConfMap["pod3"].IFace)
	if err != nil {
		t.Error(err)
	}

	confs, err = pn.List()
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, c := range confs {
		if c.ContainerId == "00f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a" {
			found = true
			if !c.IPv6.Equal(podConfMap["pod3"].IPv6) {
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
		Scope  string `json:"scope"`
	}
	type devInfo struct {
		IfName    string     `json:"ifname"`
		AddrInfos []addrInfo `json:"addr_info"`
	}
	for name, conf := range podConfMap {
		info := make([]devInfo, 0, 1)
		if conf.IPv4 != nil {
			err = pn.Update(conf.IPv4, nil, func(ipv4, ipv6 net.IP) error {
				out, err := exec.Command("ip", "-j", "addr", "show", conf.IFace).Output()
				if err != nil {
					return err
				}
				if err := json.Unmarshal(out, &info); err != nil {
					return err
				}
				return nil
			}, nil)
			if err != nil {
				t.Error(err)
			}
		} else {
			err = pn.Update(nil, conf.IPv6, func(ipv4, ipv6 net.IP) error {
				out, err := exec.Command("ip", "-j", "addr", "show", conf.IFace).Output()
				if err != nil {
					return err
				}
				if err := json.Unmarshal(out, &info); err != nil {
					return err
				}
				return nil
			}, nil)
			if err != nil {
				t.Error(err)
			}
		}

		if len(info) != 1 {
			t.Fatalf("len(info) != 1 for %s", name)
		}
		if info[0].IfName != conf.IFace {
			t.Fatalf("expected iface is %s for %s", conf.IFace, name)
		}

		for _, addr := range info[0].AddrInfos {
			if addr.Family == "inet" && addr.Scope == "global" {
				if addr.Local != conf.IPv4.String() {
					t.Fatalf("%s's inet address don't match", name)
				}
			}
			if addr.Family == "inet6" && addr.Scope == "global" {
				if addr.Local != conf.IPv6.String() {
					t.Fatalf("%s's inet6 address don't match", name)
				}
			}
		}
	}

	// destroy pod2 network
	err = pn.Destroy(podConfMap["pod2"].ContainerId, podConfMap["pod2"].IFace)
	if err != nil {
		t.Error(err)
	}

	confs, err = pn.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range confs {
		if c.ContainerId == "d8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a" {
			t.Error(`pod network 2 has not been destroyed`)
		}
	}

	// destroy should be idempotent
	err = pn.Destroy(podConfMap["pod2"].ContainerId, podConfMap["pod2"].IFace)
	if err != nil {
		t.Error(err)
	}

	// check pod2 network
	err = pn.Check(podConfMap["pod2"].ContainerId, podConfMap["pod2"].IFace)
	if err != errNotFound {
		t.Fatal("pn.Check must return error because pod2 network doesn't exist")
	}
}

func isDualStack(conf *PodNetConf) bool {
	return conf.IPv4 != nil && conf.IPv6 != nil
}
