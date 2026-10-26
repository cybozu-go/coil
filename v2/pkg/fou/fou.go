package fou

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sync"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/vishvananda/netlink"
)

// Prefixes for Foo-over-UDP tunnel link names
const (
	FoU4LinkPrefix = "fou4_"
	FoU6LinkPrefix = "fou6_"
)

// Names of the flow based IPIP tunnel devices created for NAT clients.
// See setupFlowBasedIP4TunDevice.
const (
	FlowBasedIP4TunDevice = "coil_ipip4"
	FlowBasedIP6TunDevice = "coil_ipip6"
)

const fouDummy = "fou-dummy"

func fouName(addr net.IP) string {
	if v4 := addr.To4(); v4 != nil {
		return fmt.Sprintf("%s%x", FoU4LinkPrefix, []byte(v4))
	}

	hash := sha1.Sum([]byte(addr))
	return fmt.Sprintf("%s%x", FoU6LinkPrefix, hash[:4])
}

func modProbe(module string) error {
	out, err := exec.Command("/sbin/modprobe", module).CombinedOutput()
	if err != nil {
		return fmt.Errorf("modprobe %s failed with %w: %s", module, err, string(out))
	}
	return nil
}

// FoUTunnel represents the interface for Foo-over-UDP tunnels.
// Methods are idempotent; i.e. they can be called multiple times.
type FoUTunnel interface {
	// Init starts FoU listening socket.
	Init() error

	// IsInitialized checks if this FoUTunnel has been initialized
	IsInitialized() bool

	// AddPeer setups tunnel devices to the given peer and returns them.
	// If FoUTunnel does not setup for the IP family of the given address,
	// this returns ErrIPFamilyMismatch error.
	AddPeer(net.IP, bool) (netlink.Link, error)

	// DelPeer deletes tunnel for the peer, if any.
	DelPeer(net.IP) error
}

// tunnelRole represents which side of the egress NAT a FoUTunnel works for.
type tunnelRole int

const (
	// roleClient is for NAT clients, i.e. Pods using Egress.
	roleClient tunnelRole = iota
	// roleServer is for NAT servers, i.e. egress Pods.
	roleServer
)

// NewClientFoUTunnel creates a new FoUTunnel for NAT clients.
//
// The tunnel sets up flow based IPIP tunnel devices to receive returning
// packets from egress Pods, and does not enable IP forwarding.
// See setupFlowBasedIP4TunDevice for details.
//
// port is the UDP port to receive FoU packets.
// localIPv4 is the local IPv4 address of the IPIP tunnel.  This can be nil.
// localIPv6 is the same as localIPv4 for IPv6.
func NewClientFoUTunnel(port int, localIPv4, localIPv6 net.IP, logFunc func(string)) FoUTunnel {
	return newFoUTunnel(roleClient, port, localIPv4, localIPv6, logFunc)
}

// NewServerFoUTunnel creates a new FoUTunnel for NAT servers.
//
// The tunnel enables IP forwarding to route packets from clients, and does not
// set up flow based IPIP tunnel devices so that only packets from registered
// peers are decapsulated.
//
// The parameters are the same as NewClientFoUTunnel.
func NewServerFoUTunnel(port int, localIPv4, localIPv6 net.IP, logFunc func(string)) FoUTunnel {
	return newFoUTunnel(roleServer, port, localIPv4, localIPv6, logFunc)
}

func newFoUTunnel(role tunnelRole, port int, localIPv4, localIPv6 net.IP, logFunc func(string)) FoUTunnel {
	if localIPv4 != nil && localIPv4.To4() == nil {
		panic("invalid IPv4 address")
	}
	if localIPv6 != nil && localIPv6.To4() != nil {
		panic("invalid IPv6 address")
	}
	return &fouTunnel{
		role:    role,
		port:    port,
		local4:  localIPv4,
		local6:  localIPv6,
		logFunc: logFunc,
	}
}

type fouTunnel struct {
	role    tunnelRole
	port    int
	local4  net.IP
	local6  net.IP
	logFunc func(string)

	mu sync.Mutex
}

func (t *fouTunnel) Init() error {
	// avoid double initialization in case the program restarts
	_, err := netlink.LinkByName(fouDummy)
	if err == nil {
		return nil
	}
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return err
	}

	if t.local4 != nil {
		if err := modProbe("fou"); err != nil {
			return fmt.Errorf("failed to load fou module: %w", err)
		}
		err := netlink.FouAdd(netlink.Fou{
			Family:    netlink.FAMILY_V4,
			Protocol:  4, // IPv4 over IPv4 (so-called IPIP)
			Port:      t.port,
			EncapType: netlink.FOU_ENCAP_DIRECT,
		})
		if err != nil {
			return fmt.Errorf("netlink: fou add failed: %w", err)
		}
		if _, err := sysctl.Sysctl("net.ipv4.conf.default.rp_filter", "0"); err != nil {
			return fmt.Errorf("setting net.ipv4.conf.default.rp_filter=0 failed: %w", err)
		}
		if _, err := sysctl.Sysctl("net.ipv4.conf.all.rp_filter", "0"); err != nil {
			return fmt.Errorf("setting net.ipv4.conf.all.rp_filter=0 failed: %w", err)
		}
		// NAT clients only send and receive their own packets.
		if t.role == roleServer {
			if err := ip.EnableIP4Forward(); err != nil {
				return fmt.Errorf("failed to enable IPv4 forwarding: %w", err)
			}
		}
	}
	if t.local6 != nil {
		if err := modProbe("fou6"); err != nil {
			return fmt.Errorf("failed to load fou6 module: %w", err)
		}
		err := netlink.FouAdd(netlink.Fou{
			Family:    netlink.FAMILY_V6,
			Protocol:  41, // IPv6 over IPv6 (so-called SIT)
			Port:      t.port,
			EncapType: netlink.FOU_ENCAP_DIRECT,
		})
		if err != nil {
			return fmt.Errorf("netlink: fou add failed: %w", err)
		}
		if t.role == roleServer {
			if err := ip.EnableIP6Forward(); err != nil {
				return fmt.Errorf("failed to enable IPv6 forwarding: %w", err)
			}
		}

		// avoid any existing DROP rule by rpfilter extension.
		// NB: commented as this runs in a Pod network namespace and there should be no rules.
		// if err := ipt.Insert("raw", "PREROUTING", 1, "-j", "ACCEPT"); err != nil {
		// 	return err
		// }
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = fouDummy
	return netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs})
}

func (t *fouTunnel) IsInitialized() bool {
	initialized := false
	_, err := netlink.LinkByName(fouDummy)
	if err == nil {
		initialized = true
	}
	return initialized
}

func (t *fouTunnel) AddPeer(addr net.IP, sportAuto bool) (netlink.Link, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if v4 := addr.To4(); v4 != nil {
		return t.addPeer4(v4, sportAuto)
	}
	return t.addPeer6(addr, sportAuto)
}

func (t *fouTunnel) addPeer4(addr net.IP, sportAuto bool) (netlink.Link, error) {
	if t.local4 == nil {
		return nil, ErrIPFamilyMismatch
	}

	linkName, err := t.addOrRecreatePeer4(addr, sportAuto)
	if err != nil {
		return nil, err
	}

	if t.role == roleClient {
		if err := setupFlowBasedIP4TunDevice(); err != nil {
			return nil, fmt.Errorf("netlink: failed to setup ipip device: %w", err)
		}
	}

	return netlink.LinkByName(linkName)
}

func (t *fouTunnel) addOrRecreatePeer4(addr net.IP, sportAuto bool) (string, error) {
	linkName := fouName(addr)

	link, err := netlink.LinkByName(linkName)
	if err == nil {
		iptun, ok := link.(*netlink.Iptun)
		if !ok {
			return "", fmt.Errorf("link is not IPTun: %T", link)
		}

		encapSport := uint16(t.port)
		if sportAuto {
			encapSport = 0
		}

		if encapSport != iptun.EncapSport {
			// netlink.LinkModify doesn't support updating the encap sport setting (operation not supported),
			// So we recreate the fou link.
			if t.logFunc != nil {
				t.logFunc(fmt.Sprintf("removing a fou tunnel device link: %s", linkName))
			}
			if err := netlink.LinkDel(link); err != nil {
				return "", fmt.Errorf("netlink: failed to delete fou link: %w", err)
			}
		} else {
			return linkName, nil
		}
	} else if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return "", fmt.Errorf("netlink: failed to get link: %w", err)
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = linkName
	encapSport := uint16(t.port)
	if sportAuto {
		encapSport = 0
	}
	link = &netlink.Iptun{
		LinkAttrs:  attrs,
		Ttl:        225,
		EncapType:  netlink.FOU_ENCAP_DIRECT,
		EncapDport: uint16(t.port),
		EncapSport: encapSport,
		Remote:     addr,
		Local:      t.local4,
	}

	if t.logFunc != nil {
		t.logFunc(fmt.Sprintf("add a new FoU device: %s", linkName))
	}
	if err := netlink.LinkAdd(link); err != nil {
		return "", fmt.Errorf("netlink: failed to add fou link: %w", err)
	}

	return linkName, nil
}

func (t *fouTunnel) addPeer6(addr net.IP, sportAuto bool) (netlink.Link, error) {
	if t.local6 == nil {
		return nil, ErrIPFamilyMismatch
	}

	linkName, err := t.addOrRecreatePeer6(addr, sportAuto)
	if err != nil {
		return nil, err
	}

	if t.role == roleClient {
		if err := setupFlowBasedIP6TunDevice(); err != nil {
			return nil, fmt.Errorf("netlink: failed to setup ipip device: %w", err)
		}
	}

	return netlink.LinkByName(linkName)
}

func (t *fouTunnel) addOrRecreatePeer6(addr net.IP, sportAuto bool) (string, error) {
	linkName := fouName(addr)

	link, err := netlink.LinkByName(linkName)
	if err == nil {
		ip6tnl, ok := link.(*netlink.Ip6tnl)
		if !ok {
			return "", fmt.Errorf("link is not Ip6tnl: %T", link)
		}

		encapSport := uint16(t.port)
		if sportAuto {
			encapSport = 0
		}

		if encapSport != ip6tnl.EncapSport {
			if err := netlink.LinkDel(link); err != nil {
				return "", fmt.Errorf("netlink: failed to delete fou6 link: %w", err)
			}
		} else {
			return linkName, nil
		}
	} else if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return "", fmt.Errorf("netlink: failed to get link: %w", err)
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = linkName
	encapSport := uint16(t.port)
	if sportAuto {
		encapSport = 0
	}
	link = &netlink.Ip6tnl{
		LinkAttrs:  attrs,
		Ttl:        225,
		EncapType:  netlink.FOU_ENCAP_DIRECT,
		EncapDport: uint16(t.port),
		EncapSport: encapSport,
		Remote:     addr,
		Local:      t.local6,
	}
	if t.logFunc != nil {
		t.logFunc(fmt.Sprintf("add a new FoU device: %s", linkName))
	}
	if err := netlink.LinkAdd(link); err != nil {
		return "", fmt.Errorf("netlink: failed to add fou6 link: %w", err)
	}

	return linkName, nil
}

// setupFlowBasedIP[4,6]TunDevice creates an IPv4 or IPv6 tunnel device
//
// This flow based IPIP tunnel device is used to decapsulate packets from
// the router Pods.
//
// It is needed only for NAT clients.  A client sends packets to the Service's
// ClusterIP, but the returning packets come from the router Pod's IP address,
// so they do not match the IPIP tunnel device configured for the ClusterIP.
// Since this device accepts packets from any source, NAT servers must not
// create it; packets from clients always match the per-peer tunnel devices.
// On NAT servers, the fallback devices are left as they are.  They are down
// and never used for decapsulation.
//
// Calling this function may result in tunl0 (v4) or ip6tnl0 (v6)
// fallback interface being renamed to coil_tunl or coil_ip6tnl.
// This is to communicate to the user that this plugin has taken
// control of the encapsulation stack on the netns, as it currently
// doesn't explicitly support sharing it with other tools/CNIs.
// Fallback devices are left unused for production traffic.
// Only devices that were explicitly created are used.
//
// This fallback interface is present as a result of loading the
// ipip and ip6_tunnel kernel modules by fou tunnel interfaces.
// These are catch-all interfaces for the ipip decapsulation stack.
// By default, these interfaces will be created in new network namespaces,
// but this behavior can be disabled by setting net.core.fb_tunnels_only_for_init_net = 2.
func setupFlowBasedIP4TunDevice() error {
	ipip4Device := FlowBasedIP4TunDevice
	// Set up IPv4 tunnel device if requested.
	if err := setupDevice(&netlink.Iptun{
		Name:      ipip4Device,
		FlowBased: true,
	}); err != nil {
		return fmt.Errorf("creating %s: %w", ipip4Device, err)
	}

	// Rename fallback device created by potential kernel module load after
	// creating tunnel interface.
	if err := renameDevice("tunl0", "coil_tunl"); err != nil {
		return fmt.Errorf("renaming fallback device %s: %w", "tunl0", err)
	}

	return nil
}

// See setupFlowBasedIP4TunDevice
func setupFlowBasedIP6TunDevice() error {
	ipip6Device := FlowBasedIP6TunDevice

	// Set up IPv6 tunnel device if requested.
	if err := setupDevice(&netlink.Ip6tnl{
		Name:      ipip6Device,
		FlowBased: true,
	}); err != nil {
		return fmt.Errorf("creating %s: %w", ipip6Device, err)
	}

	// Rename fallback device created by potential kernel module load after
	// creating tunnel interface.
	if err := renameDevice("ip6tnl0", "coil_ip6tnl"); err != nil {
		return fmt.Errorf("renaming fallback device %s: %w", "tunl0", err)
	}

	return nil
}

// setupDevice creates and configures a device based on the given netlink attrs.
func setupDevice(link netlink.Link) error {
	name := link.Attrs().Name

	// Reuse existing tunnel interface created by previous runs.
	l, err := netlink.LinkByName(name)
	if err != nil {
		if _, ok := errors.AsType[netlink.LinkNotFoundError](err); !ok {
			return err
		}

		if err := netlink.LinkAdd(link); err != nil {
			return fmt.Errorf("netlink: failed to create device %s: %w", name, err)
		}

		// Fetch the link we've just created.
		l, err = netlink.LinkByName(name)
		if err != nil {
			return fmt.Errorf("netlink: failed to retrieve created device %s: %w", name, err)
		}
	}

	if err := configureDevice(l); err != nil {
		return fmt.Errorf("failed to set up device %s: %w", l.Attrs().Name, err)
	}

	return nil
}

// configureDevice puts the given link into the up state
func configureDevice(link netlink.Link) error {
	ifName := link.Attrs().Name

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("netlink: failed to set link %s up: %w", ifName, err)
	}
	return nil
}

// renameDevice renames a network device from and to a given value. Returns nil
// if the device does not exist.
func renameDevice(from, to string) error {
	link, err := netlink.LinkByName(from)
	if err != nil {
		//nolint:nilerr
		return nil
	}

	if err := netlink.LinkSetName(link, to); err != nil {
		return fmt.Errorf("netlink: failed to rename device %s to %s: %w", from, to, err)
	}

	return nil
}

func (t *fouTunnel) DelPeer(addr net.IP) error {
	linkName := fouName(addr)

	link, err := netlink.LinkByName(linkName)
	if err == nil {
		return netlink.LinkDel(link)
	}

	if _, ok := err.(netlink.LinkNotFoundError); ok {
		return nil
	}
	return err
}
