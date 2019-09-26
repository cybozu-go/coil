// Copyright 2015 CNI authors
// Copyright 2019 Cybozu
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cni

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

var (
	linkLocalNode = net.ParseIP("169.254.1.1")
)

func ipToIPNet(ip net.IP) *net.IPNet {
	bits := 32
	if ip.To4() == nil {
		bits = 128
	}
	return &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(bits, bits),
	}
}

// setupVeth does:
// 1. creates a veth pair in the container NS,
// 2. moves one side of the pair to the host NS, and
// 3. fill "Interface" objects which will be used in the plugin result, and
// 4. sets a link local address to the host-side veth.
func setupVeth(netns ns.NetNS, ifName, namespace, podname string) (*current.Interface, *current.Interface, error) {
	contIface := new(current.Interface)
	hostIface := new(current.Interface)

	err := netns.Do(func(hostNS ns.NetNS) error {
		hostVeth, containerVeth, err := setupVethPair(ifName, namespace, podname, 0, hostNS)
		if err != nil {
			return err
		}
		contIface.Name = containerVeth.Name
		contIface.Mac = containerVeth.HardwareAddr.String()
		contIface.Sandbox = netns.Path()
		hostIface.Name = hostVeth.Name
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	hostVeth, err := netlink.LinkByName(hostIface.Name)
	if err != nil {
		return nil, nil, err
	}
	hostIface.Mac = hostVeth.Attrs().HardwareAddr.String()

	addr := &netlink.Addr{IPNet: ipToIPNet(linkLocalNode), Scope: unix.RT_SCOPE_LINK, Label: ""}
	err = netlink.AddrAdd(hostVeth, addr)
	if err != nil {
		return nil, nil, err
	}

	return hostIface, contIface, nil
}

func setupVethPair(contVethName, namespace, podname string, mtu int, hostNS ns.NetNS) (net.Interface, net.Interface, error) {
	hostVethName, contVeth, err := makeVeth(contVethName, namespace, podname, mtu)
	if err != nil {
		return net.Interface{}, net.Interface{}, err
	}

	if err = netlink.LinkSetUp(contVeth); err != nil {
		return net.Interface{}, net.Interface{}, fmt.Errorf("failed to set %q up: %v", contVethName, err)
	}
	if injectFailures {
		panicForFirstRun("AfterMakingVethPairInContainerNS")
	}

	hostVeth, err := netlink.LinkByName(hostVethName)
	if err != nil {
		return net.Interface{}, net.Interface{}, fmt.Errorf("failed to lookup %q: %v", hostVethName, err)
	}

	err = hostNS.Do(func(_ ns.NetNS) error {
		return deleteVethIfExists(hostVethName)
	})
	if err != nil {
		return net.Interface{}, net.Interface{}, fmt.Errorf("failed to delete host veth %q: %v", hostVethName, err)
	}

	if err = netlink.LinkSetNsFd(hostVeth, int(hostNS.Fd())); err != nil {
		return net.Interface{}, net.Interface{}, fmt.Errorf("failed to move veth to host netns: %v", err)
	}

	err = hostNS.Do(func(_ ns.NetNS) error {
		hostVeth, err = netlink.LinkByName(hostVethName)
		if err != nil {
			return fmt.Errorf("failed to lookup %q in %q: %v", hostVethName, hostNS.Path(), err)
		}

		if err = netlink.LinkSetUp(hostVeth); err != nil {
			return fmt.Errorf("failed to set %q up: %v", hostVethName, err)
		}
		return nil
	})
	if err != nil {
		return net.Interface{}, net.Interface{}, err
	}
	return ifaceFromNetlinkLink(hostVeth), ifaceFromNetlinkLink(contVeth), nil
}

func ifaceFromNetlinkLink(l netlink.Link) net.Interface {
	a := l.Attrs()
	return net.Interface{
		Index:        a.Index,
		MTU:          a.MTU,
		Name:         a.Name,
		HardwareAddr: a.HardwareAddr,
		Flags:        a.Flags,
	}
}

func generateHostVethName(prefix, namespace, podname string) string {
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s.%s", namespace, podname)))
	return fmt.Sprintf("%s%s", prefix, hex.EncodeToString(h.Sum(nil))[:11])
}

func makeVeth(name, namespace, podname string, mtu int) (peerName string, veth netlink.Link, err error) {
	peerName = generateHostVethName("veth", namespace, podname)

	err = deleteVethIfExists(peerName)
	if err != nil {
		err = fmt.Errorf("failed to delete peer veth %q: %v", peerName, err)
		return
	}

	veth, err = makeVethPair(name, peerName, mtu)
	if os.IsExist(err) {
		err = fmt.Errorf("container veth name provided (%v) already exists", name)
		return
	}
	return
}

func makeVethPair(name, peer string, mtu int) (netlink.Link, error) {
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name:  name,
			Flags: net.FlagUp,
			MTU:   mtu,
		},
		PeerName: peer,
	}

	if err := netlink.LinkAdd(veth); err != nil {
		return nil, err
	}
	// Re-fetch the link to get its creation-time parameters, e.g. index and mac
	veth2, err := netlink.LinkByName(name)
	if err != nil {
		netlink.LinkDel(veth) // try and clean up the link if possible.
		return nil, err
	}

	return veth2, nil
}

func deleteVethIfExists(name string) error {
	oldVeth, err := netlink.LinkByName(name)
	switch err.(type) {
	case nil:
		return netlink.LinkDel(oldVeth)
	case netlink.LinkNotFoundError:
		return nil
	default:
		return err
	}
}

// addRouteInHost does:
// 1. "ip route add <assinged IP address>/32 dev <host-side veth> scope link".
func addRouteInHost(dst net.IP, devName string) error {
	dev, err := netlink.LinkByName(devName)
	if err != nil {
		return err
	}

	route := &netlink.Route{
		LinkIndex: dev.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       ipToIPNet(dst),
	}
	return netlink.RouteAdd(route)
}

// addRouteInContainer does:
// 1. "ip route add <host-side link local address>/32 dev eth0 scope link", and
// 2. "ip route add default via <host-side link local address> scope global".
func addRouteInContainer(devName string) error {
	dev, err := netlink.LinkByName(devName)
	if err != nil {
		return err
	}

	hostRoute := &netlink.Route{
		LinkIndex: dev.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       ipToIPNet(linkLocalNode),
	}

	err = netlink.RouteAdd(hostRoute)
	if err != nil {
		return err
	}

	_, defaultgw, err := net.ParseCIDR("0.0.0.0/0")
	if err != nil {
		return err
	}
	defaultRoute := &netlink.Route{
		Scope: netlink.SCOPE_UNIVERSE,
		Dst:   defaultgw,
		Gw:    linkLocalNode,
	}

	return netlink.RouteAdd(defaultRoute)
}

// configureInterface does:
// 1. assigns an IP address,
// 2. adds route to the host node, and
// 3. adds default route, all in the container.
func configureInterface(netns ns.NetNS, ifName string, result *current.Result) error {
	return netns.Do(func(_ ns.NetNS) error {
		err := ipam.ConfigureIface(ifName, result)
		if err != nil {
			return err
		}

		err = addRouteInContainer(ifName)
		if err != nil {
			return err
		}

		return nil
	})
}

// Add adds an IP address to a container.
func Add(args *skel.CmdArgs) error {
	var success bool

	conf := new(PluginConf)
	err := json.Unmarshal(args.StdinData, conf)
	if err != nil {
		return err
	}
	coildURL, err := url.Parse(conf.CoildURL)
	if err != nil {
		return err
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return err
	}
	defer netns.Close()

	kv := parseArgs(args.Args)
	podNS, podName, err := getPodInfo(kv)
	if err != nil {
		return err
	}

	hostInterface, containerInterface, err := setupVeth(netns, args.IfName, podNS, podName)
	if err != nil {
		return err
	}
	if injectFailures {
		panicForFirstRun("AfterMakingVethPair")
	}

	ip, err := getIPFromCoild(coildURL, podNS, podName, args.ContainerID)
	if err != nil {
		return err
	}
	defer func() {
		if !success {
			returnIPToCoild(coildURL, podNS, podName, args.ContainerID)
		}
	}()
	if injectFailures {
		panicForFirstRun("AfterGettingIP")
	}

	err = addRouteInHost(ip, hostInterface.Name)
	if err != nil {
		return err
	}

	result := &current.Result{}
	result.Interfaces = []*current.Interface{hostInterface, containerInterface}
	result.IPs = []*current.IPConfig{
		{
			Version:   "4",
			Interface: current.Int(1),
			Address:   *ipToIPNet(ip),
		},
	}

	err = configureInterface(netns, args.IfName, result)
	if err != nil {
		return err
	}
	if injectFailures {
		panicForFirstRun("AfterConfiguringInterfaces")
	}

	success = true
	return types.PrintResult(result, conf.CNIVersion)
}
