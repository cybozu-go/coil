package cni

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

// PluginConf is configuration for this plugin.
type PluginConf struct {
	types.NetConf
	HostInterface string `json:"host-interface"`
	//Table     int    `json:"table"`
}

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

// callIPAMAdd calls IPAM plugin to reserve an IP address for the container.
func callIPAMAdd(ipamType string, stdinData []byte) (*current.Result, error) {
	var success bool

	r, err := ipam.ExecAdd(ipamType, stdinData)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			callIPAMDel(ipamType, stdinData)
		}
	}()

	result, err := current.NewResultFromResult(r)
	if err != nil {
		return nil, err
	}

	if len(result.IPs) == 0 {
		return nil, errors.New("IPAM plugin returned missing IP config")
	}

	success = true
	return result, nil
}

// callIPAMDel releases the reseved IP address in case of failure.
func callIPAMDel(ipamType string, stdinData []byte) {
	os.Setenv("CNI_COMMAND", "DEL")
	ipam.ExecDel(ipamType, stdinData)
	os.Setenv("CNI_COMMAND", "ADD")
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
func addRouteInContainer(host net.IP, devName string) error {
	hostNet := &net.IPNet{
		IP:   host,
		Mask: net.CIDRMask(32, 32),
	}

	dev, err := netlink.LinkByName(devName)
	if err != nil {
		return err
	}

	hostRoute := &netlink.Route{
		LinkIndex: dev.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       hostNet,
	}

	err = netlink.RouteAdd(hostRoute)
	if err != nil {
		return err
	}

	defaultNet := &net.IPNet{
		IP:   net.IPv4(0, 0, 0, 0).To4(),
		Mask: net.CIDRMask(0, 32),
	}

	route := &netlink.Route{
		LinkIndex: dev.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst:       defaultNet,
		Gw:        host,
	}
	return netlink.RouteAdd(route)
}

// configureInterface (1) assigns reserved IP addresses, (2) adds route to the host node,
// and (3) adds default route, all in the container.
func configureInterface(netns ns.NetNS, ifName, hostIfName string, result *current.Result) error {
	hostLink, err := netlink.LinkByName(hostIfName)
	if err != nil {
		return err
	}

	addrs, err := netlink.AddrList(hostLink, netlink.FAMILY_V4)
	if err != nil {
		return err
	}

	if len(addrs) == 0 {
		return fmt.Errorf("host interface %s has no IP address", hostIfName)
	}

	return netns.Do(func(_ ns.NetNS) error {
		err = ipam.ConfigureIface(ifName, result)
		if err != nil {
			return err
		}

		err = addRouteInContainer(addrs[0].IP, ifName)
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

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return err
	}
	defer netns.Close()

	hostInterface, containerInterface, err := setupVeth(netns, args.IfName)
	if err != nil {
		return err
	}

	result, err := callIPAMAdd(conf.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}
	defer func() {
		if !success {
			callIPAMDel(conf.IPAM.Type, args.StdinData)
		}
	}()

	result.Interfaces = []*current.Interface{hostInterface, containerInterface}
	for _, ipc := range result.IPs {
		ipc.Interface = current.Int(1) // point to containerInterface
		ipc.Address = net.IPNet{
			IP:   ipc.Address.IP,
			Mask: net.CIDRMask(32, 32), // we need "/32" address regardless of "subnet" in host-local IPAM config
		}
		ipc.Gateway = nil // host-local IPAM returns "a.b.c.1" as GW, but we don't use it

		err = addRouteInHost(ipc.Address.IP, hostInterface.Name)
		if err != nil {
			return err
		}
	}

	err = configureInterface(netns, args.IfName, conf.HostInterface, result)
	if err != nil {
		return err
	}

	success = true
	return types.PrintResult(result, conf.CNIVersion)
}
