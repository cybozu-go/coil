package cni

import (
	"encoding/json"
	"net"
	"net/url"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

// setupVeth (1) creates a veth pair in the container NS, (2) moves one side of the pair to the host NS,
// and (3) fill "Interface" objects which will be used in the plugin result.
func setupVeth(netns ns.NetNS, ifName string) (*current.Interface, *current.Interface, error) {
	contIface := new(current.Interface)
	hostIface := new(current.Interface)

	err := netns.Do(func(hostNS ns.NetNS) error {
		hostVeth, containerVeth, err := ip.SetupVeth(ifName, 0, hostNS)
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

	return hostIface, contIface, nil
}

// addRouteInHost does "ip route add <assinged IP address>/32 dev <host-side veth> scope link".
func addRouteInHost(dst net.IP, devName string) error {
	dstNet := &net.IPNet{
		IP:   dst,
		Mask: net.CIDRMask(32, 32),
	}

	dev, err := netlink.LinkByName(devName)
	if err != nil {
		return err
	}

	route := &netlink.Route{
		LinkIndex: dev.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       dstNet,
	}
	return netlink.RouteAdd(route)
}

// addRouteInContainer does "ip route add <host IP address>/32 dev <interface in container> scope link"
// and "ip route add default via <host IP address> scope global".
func addRouteInContainer(devName string) error {
	dev, err := netlink.LinkByName(devName)
	if err != nil {
		return err
	}

	_, defaultgw, err := net.ParseCIDR("0.0.0.0/0")
	if err != nil {
		return err
	}
	hostRoute := &netlink.Route{
		LinkIndex: dev.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst:       defaultgw,
	}

	return netlink.RouteAdd(hostRoute)
}

// configureInterface (1) assigns reserved IP addresses, (2) adds route to the host node,
// and (3) adds default route, all in the container.
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

	hostInterface, containerInterface, err := setupVeth(netns, args.IfName)
	if err != nil {
		return err
	}

	kv := parseArgs(args.Args)
	podNS, podName, err := getPodInfo(kv)
	if err != nil {
		return err
	}

	ip, err := getIPFromCoild(coildURL, podNS, podName)
	if err != nil {
		return err
	}
	defer func() {
		if !success {
			returnIPToCoild(coildURL, podNS, podName)
		}
	}()

	err = addRouteInHost(ip, hostInterface.Name)
	if err != nil {
		return err
	}

	result := &current.Result{}
	result.Interfaces = []*current.Interface{hostInterface, containerInterface}
	result.IPs = []*current.IPConfig{
		&current.IPConfig{
			Version:   "4",
			Interface: current.Int(1),
			Address:   net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)},
		},
	}

	err = configureInterface(netns, args.IfName, result)
	if err != nil {
		return err
	}

	success = true
	return types.PrintResult(result, conf.CNIVersion)
}
