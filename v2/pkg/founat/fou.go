package founat

import (
	"crypto/sha1"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"sync"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
)

// Prefixes for Foo-over-UDP tunnel link names
const (
	FoU4LinkPrefix = "fou4_"
	FoU6LinkPrefix = "fou6_"
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

	// AddPeer setups tunnel devices to the given peer and returns them.
	// If FoUTunnel does not setup for the IP family of the given address,
	// this returns ErrIPFamilyMismatch error.
	AddPeer(net.IP) (netlink.Link, error)

	// DelPeer deletes tunnel for the peer, if any.
	DelPeer(net.IP) error
}

// NewFoUTunnel creates a new FoUTunnel.
// port is the UDP port to receive FoU packets.
// localIPv4 is the local IPv4 address of the IPIP tunnel.  This can be nil.
// localIPv6 is the same as localIPv4 for IPv6.
func NewFoUTunnel(port int, localIPv4, localIPv6 net.IP) FoUTunnel {
	if localIPv4 != nil && localIPv4.To4() == nil {
		panic("invalid IPv4 address")
	}
	if localIPv6 != nil && localIPv6.To4() != nil {
		panic("invalid IPv6 address")
	}
	return &fouTunnel{
		port:   port,
		local4: localIPv4,
		local6: localIPv6,
	}
}

type fouTunnel struct {
	port   int
	local4 net.IP
	local6 net.IP

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
		if err := ip.EnableIP4Forward(); err != nil {
			return fmt.Errorf("failed to enable IPv4 forwarding: %w", err)
		}

		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
		if err != nil {
			return err
		}
		// workaround for kube-proxy's double NAT problem
		rulespec := []string{
			"-p", "udp", "--dport", strconv.Itoa(t.port), "-j", "CHECKSUM", "--checksum-fill",
		}
		if err := ipt.Insert("mangle", "POSTROUTING", 1, rulespec...); err != nil {
			return fmt.Errorf("failed to setup mangle table: %w", err)
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
		if err := ip.EnableIP6Forward(); err != nil {
			return fmt.Errorf("failed to enable IPv6 forwarding: %w", err)
		}

		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
		if err != nil {
			return err
		}
		// workaround for kube-proxy's double NAT problem
		rulespec := []string{
			"-p", "udp", "--dport", strconv.Itoa(t.port), "-j", "CHECKSUM", "--checksum-fill",
		}
		if err := ipt.Insert("mangle", "POSTROUTING", 1, rulespec...); err != nil {
			return fmt.Errorf("failed to setup mangle table: %w", err)
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

func (t *fouTunnel) AddPeer(addr net.IP) (netlink.Link, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if v4 := addr.To4(); v4 != nil {
		return t.addPeer4(v4)
	}
	return t.addPeer6(addr)
}

func (t *fouTunnel) addPeer4(addr net.IP) (netlink.Link, error) {
	if t.local4 == nil {
		return nil, ErrIPFamilyMismatch
	}

	linkName := fouName(addr)

	link, err := netlink.LinkByName(linkName)
	if err == nil {
		return link, nil
	}
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return nil, fmt.Errorf("netlink: failed to get link: %w", err)
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = linkName
	attrs.Flags = net.FlagUp
	link = &netlink.Iptun{
		LinkAttrs:  attrs,
		Ttl:        225,
		EncapType:  netlink.FOU_ENCAP_DIRECT,
		EncapDport: uint16(t.port),
		EncapSport: uint16(t.port),
		Remote:     addr,
		Local:      t.local4,
	}
	if err := netlink.LinkAdd(link); err != nil {
		return nil, fmt.Errorf("netlink: failed to add fou link: %w", err)
	}

	return netlink.LinkByName(linkName)
}

func (t *fouTunnel) addPeer6(addr net.IP) (netlink.Link, error) {
	if t.local6 == nil {
		return nil, ErrIPFamilyMismatch
	}

	linkName := fouName(addr)

	link, err := netlink.LinkByName(linkName)
	if err == nil {
		return link, nil
	}
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return nil, fmt.Errorf("netlink: failed to get link: %w", err)
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = linkName
	attrs.Flags = net.FlagUp
	link = &netlink.Ip6tnl{
		LinkAttrs:  attrs,
		Ttl:        225,
		EncapType:  netlink.FOU_ENCAP_DIRECT,
		EncapDport: uint16(t.port),
		EncapSport: uint16(t.port),
		Remote:     addr,
		Local:      t.local6,
	}
	if err := netlink.LinkAdd(link); err != nil {
		return nil, fmt.Errorf("netlink: failed to add fou6 link: %w", err)
	}

	return netlink.LinkByName(linkName)
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
