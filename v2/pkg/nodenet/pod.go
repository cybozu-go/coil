package nodenet

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/cybozu-go/netutil"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	corev1 "k8s.io/api/core/v1"
)

var (
	errNotFound = errors.New("not found")

	defaultGWv4 = &net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)}
	defaultGWv6 = &net.IPNet{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)}
)

// SetupHook is a signature of hook function for PodNetwork.Setup
type SetupHook func(ipv4, ipv6 net.IP) error

// PodNetConf holds configuration parameters for a Pod network
type PodNetConf struct {
	PoolName     string
	ContainerId  string
	IFace        string
	IPv4         net.IP
	IPv6         net.IP
	HostVethName string
}

// PodNetwork represents an interface to configure container networking.
type PodNetwork interface {
	// Init initializes the host network.
	Init() error

	// SetupIPAM connects the host network and the container network with a veth pair.
	// `nsPath` is the container network namespace's (possibly bind-mounted) file.
	// If `hook` is non-nil, it is called in the Pod network.
	SetupIPAM(nsPath, podName, podNS string, conf *PodNetConf) (*current.Result, error)

	// SetupEgress configures egress for container.
	// `nsPath` is the container network namespace's (possibly bind-mounted) file.
	// If `hook` is non-nil, it is called in the Pod network.
	SetupEgress(nsPath string, conf *PodNetConf, hook SetupHook) error

	// Update updates the container network configuration
	// Currently, it only updates configuration using a SetupHook, e.g. NAT setting
	Update(podIPv4, podIPv6 net.IP, hook SetupHook, pod *corev1.Pod) error

	// Check checks the pod network's status.
	Check(containerId, iface string) error

	// Destroy disconnects the container network by deleting the veth pair.
	// IPv4 and IPv6 in conf can be left nil.
	Destroy(containerId, iface string) error

	// List returns a list of already setup network configurations.
	List() ([]*PodNetConf, error)
}

// NewPodNetwork creates a PodNetwork
func NewPodNetwork(podTableID, podRulePrio, protocolId int, hostIPv4, hostIPv6 net.IP, compatCalico, registerFromMain bool, log logr.Logger, enableIPAM bool) PodNetwork {
	return &podNetwork{
		podTableId:       podTableID,
		podRulePrio:      podRulePrio,
		protocolId:       netlink.RouteProtocol(protocolId),
		hostIPv4:         hostIPv4,
		hostIPv6:         hostIPv6,
		compatCalico:     compatCalico,
		registerFromMain: registerFromMain,
		log:              log,
		enableIPAM:       enableIPAM,
	}
}

type podNetwork struct {
	podTableId       int
	podRulePrio      int
	protocolId       netlink.RouteProtocol
	mtu              int
	hostIPv4         net.IP
	hostIPv6         net.IP
	compatCalico     bool
	registerFromMain bool
	log              logr.Logger
	enableIPAM       bool

	mu sync.Mutex
}

func GenAlias(conf *PodNetConf, id string) string {
	return fmt.Sprintf("COIL:%s:%s:%s", conf.PoolName, id, conf.IFace)
}

func parseLink(l netlink.Link) *PodNetConf {
	cols := strings.Split(l.Attrs().Alias, ":")
	if len(cols) != 4 {
		return nil
	}
	if cols[0] != "COIL" {
		return nil
	}

	return &PodNetConf{
		PoolName:    cols[1],
		ContainerId: cols[2],
		IFace:       cols[3],
	}
}

func calicoVethName(podName, podNS string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s.%s", podNS, podName)))
	return "veth" + hex.EncodeToString(sum[:])[:11]
}

func lookup(containerId, iface string) (netlink.Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("netlink: failed to list links: %w", err)
	}

	for _, l := range links {
		c := parseLink(l)
		if c == nil {
			continue
		}

		if c.ContainerId == containerId && c.IFace == iface {
			return l, nil
		}
	}

	return nil, errNotFound
}

func (pn *podNetwork) Init() error {
	if err := ip.EnableIP4Forward(); err != nil {
		pn.log.Error(err, "warning: failed to enable IPv4 forwarding")
	}
	if err := ip.EnableIP6Forward(); err != nil {
		pn.log.Error(err, "warning: failed to enable IPv6 forwarding")
	}

	if err := pn.initRule(netlink.FAMILY_V4); err != nil {
		pn.log.Error(err, "warning: failed to init IPv4 routing rule")
	}
	if err := pn.initRule(netlink.FAMILY_V6); err != nil {
		pn.log.Error(err, "warning: failed to init IPv6 routing rule")
	}

	if mtu, err := netutil.DetectMTU(); err != nil {
		pn.log.Error(err, "warning: failed to auto-detect the host MTU")
	} else {
		pn.mtu = mtu
	}

	return nil
}

func (pn *podNetwork) initRule(family int) error {
	rules, err := netlink.RuleList(family)
	if err != nil {
		return fmt.Errorf("netlink: rule list failed: %w", err)
	}

	for _, r := range rules {
		if r.Priority == pn.podRulePrio {
			return nil
		}
	}

	r := netlink.NewRule()
	r.Family = family
	r.Table = pn.podTableId
	r.Priority = pn.podRulePrio
	if err := netlink.RuleAdd(r); err != nil {
		return fmt.Errorf("netlink: failed to add pod table rule: %w", err)
	}
	return nil
}

func (pn *podNetwork) SetupIPAM(nsPath, podName, podNS string, conf *PodNetConf) (*current.Result, error) {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	containerNS, err := ns.GetNS(nsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open netns path %s: %w", nsPath, err)
	}
	defer containerNS.Close()

	var hostIPv6 net.IP
	var hLink netlink.Link

	result := &current.Result{
		CNIVersion: current.ImplementedSpecVersion,
	}

	// cleanup garbage veth
	switch l, err := lookup(conf.ContainerId, conf.IFace); err {
	case errNotFound:
	case nil:
		// remove garbage link, if any
		if err := netlink.LinkDel(l); err != nil {
			return nil, fmt.Errorf("netlink: failed to delete broken link: %w", err)
		}
	default:
		return nil, err
	}

	// setup veth and configure IP addresses
	err = containerNS.Do(func(hostNS ns.NetNS) error {
		vethName := ""
		if pn.compatCalico {
			vethName = calicoVethName(podName, podNS)
		}
		hVeth, cVeth, err := ip.SetupVethWithName(conf.IFace, vethName, pn.mtu, "", hostNS)
		if err != nil {
			return fmt.Errorf("failed to setup veth: %w", err)
		}

		cLink, err := netlink.LinkByIndex(cVeth.Index)
		if err != nil {
			return fmt.Errorf("netlink: failed to get veth link for container: %w", err)
		}

		if err := netlink.LinkSetUp(cLink); err != nil {
			netlink.LinkDel(cLink)
			return fmt.Errorf("netlink: failed to up link for container: %w", err)
		}

		idx := 0
		if conf.IPv4 != nil {
			ipnet := netlink.NewIPNet(conf.IPv4)
			err := netlink.AddrAdd(cLink, &netlink.Addr{
				IPNet: ipnet,
				Scope: unix.RT_SCOPE_UNIVERSE,
			})
			if err != nil {
				netlink.LinkDel(cLink)
				return fmt.Errorf("netlink: failed to add an address: %w", err)
			}
			result.IPs = append(result.IPs, &current.IPConfig{
				Address:   *ipnet,
				Interface: &idx,
			})
		}

		if conf.IPv6 != nil {
			ipnet := netlink.NewIPNet(conf.IPv6)
			err := netlink.AddrAdd(cLink, &netlink.Addr{
				IPNet: ipnet,
				Scope: unix.RT_SCOPE_UNIVERSE,
			})
			if err != nil {
				netlink.LinkDel(cLink)
				return fmt.Errorf("netlink: failed to add an address: %w", err)
			}
			if err = ip.SettleAddresses(conf.IFace, 10*time.Second); err != nil {
				return fmt.Errorf("netlink: failed to settle an address: %w", err)
			}
			result.IPs = append(result.IPs, &current.IPConfig{
				Address:   *ipnet,
				Interface: &idx,
			})
		}

		result.Interfaces = []*current.Interface{
			{
				Name:    cVeth.Name,
				Mac:     cVeth.HardwareAddr.String(),
				Sandbox: nsPath,
			},
			{
				Name: hVeth.Name,
				Mac:  hVeth.HardwareAddr.String(),
			},
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// install cleanup handler upon errors
	hName := result.Interfaces[1].Name
	hLink, err = netlink.LinkByName(hName)
	if err != nil {
		return nil, fmt.Errorf("netlink: failed to look up the host-side veth: %w", err)
	}
	defer func() {
		if hLink != nil {
			netlink.LinkDel(hLink)
		}
	}()

	// give identifier as an alias of host veth
	err = netlink.LinkSetAlias(hLink, GenAlias(conf, conf.ContainerId))
	if err != nil {
		return nil, fmt.Errorf("netlink: failed to set alias: %w", err)
	}

	// setup routing on the host side
	if conf.IPv6 != nil {
		err = netlink.AddrAdd(hLink, &netlink.Addr{
			IPNet: netlink.NewIPNet(pn.hostIPv6),
			Scope: unix.RT_SCOPE_UNIVERSE,
		})
		if err != nil {
			return nil, fmt.Errorf("netlink: failed to add a host IPv6 address: %w", err)
		}
		if err = ip.SettleAddresses(hName, 10*time.Second); err != nil {
			return nil, fmt.Errorf("netlink: failed to settle a host IPv6 address: %w", err)
		}

		v6Addrs, err := netlink.AddrList(hLink, netlink.FAMILY_V6)
		if err != nil {
			return nil, fmt.Errorf("failed to get v6 addresses: %w", err)
		}
		for _, a := range v6Addrs {
			if a.Scope == unix.RT_SCOPE_LINK {
				hostIPv6 = a.IP
				break
			}
		}
		if hostIPv6 == nil {
			return nil, fmt.Errorf("failed to find link-local address of %s", hLink.Attrs().Name)
		}

		err = netlink.RouteAdd(&netlink.Route{
			Dst:       netlink.NewIPNet(conf.IPv6),
			LinkIndex: hLink.Attrs().Index,
			Scope:     netlink.SCOPE_LINK,
			Protocol:  pn.protocolId,
			Table:     pn.podTableId,
		})
		if err != nil {
			return nil, fmt.Errorf("netlink: failed to add route to IPv6 %s: %w", conf.IPv6.String(), err)
		}
	}
	if conf.IPv4 != nil {
		err = netlink.AddrAdd(hLink, &netlink.Addr{
			IPNet: netlink.NewIPNet(pn.hostIPv4),
			Scope: unix.RT_SCOPE_UNIVERSE,
		})
		if err != nil {
			return nil, fmt.Errorf("netlink: failed to add a hostIPv4 address: %w", err)
		}

		err = netlink.RouteAdd(&netlink.Route{
			Dst:       netlink.NewIPNet(conf.IPv4),
			LinkIndex: hLink.Attrs().Index,
			Scope:     netlink.SCOPE_LINK,
			Protocol:  pn.protocolId,
			Table:     pn.podTableId,
		})
		if err != nil {
			return nil, fmt.Errorf("netlink: failed to add route to IPv4 %s: %w", conf.IPv4.String(), err)
		}
	}

	// setup routing on the container side
	err = containerNS.Do(func(ns.NetNS) error {
		l, err := netlink.LinkByName(conf.IFace)
		if err != nil {
			return fmt.Errorf("netlink: failed to find link: %w", err)
		}
		if conf.IPv4 != nil {
			err := netlink.RouteAdd(&netlink.Route{
				Dst:       netlink.NewIPNet(pn.hostIPv4),
				LinkIndex: l.Attrs().Index,
				Scope:     netlink.SCOPE_LINK,
			})
			if err != nil {
				return fmt.Errorf("netlink: failed to add route to host %s: %w", pn.hostIPv4.String(), err)
			}
			err = netlink.RouteAdd(&netlink.Route{
				Dst:   defaultGWv4,
				Gw:    pn.hostIPv4,
				Scope: netlink.SCOPE_UNIVERSE,
			})
			if err != nil {
				return fmt.Errorf("netlink: failed to add default gw %s: %w", pn.hostIPv4.String(), err)
			}
		}
		if conf.IPv6 != nil {
			err = netlink.RouteAdd(&netlink.Route{
				Dst:       defaultGWv6,
				Gw:        hostIPv6,
				LinkIndex: l.Attrs().Index, // hostIPv6 is a link-local address, so this is required
				Scope:     netlink.SCOPE_UNIVERSE,
			})
			if err != nil {
				return fmt.Errorf("netlink: failed to add default gw %s: %w", hostIPv6.String(), err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	hLink = nil
	return result, nil
}

func (pn *podNetwork) SetupEgress(nsPath string, conf *PodNetConf, hook SetupHook) error {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	containerNS, err := ns.GetNS(nsPath)
	if err != nil {
		return fmt.Errorf("failed to open netns path %s: %w", nsPath, err)
	}
	defer containerNS.Close()

	// setup routing on the container side
	err = containerNS.Do(func(ns.NetNS) error {
		if hook != nil {
			return hook(conf.IPv4, conf.IPv6)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error executing hook: %w", err)
	}

	return nil
}

func (pn *podNetwork) Update(podIPv4, podIPv6 net.IP, hook SetupHook, pod *corev1.Pod) error {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	podConfigs, err := pn.list()
	if err != nil {
		return err
	}

	var netNsPath string
	for _, c := range podConfigs {
		if pn.enableIPAM {
			// When both c.IPvX and podIPvX are nil, net.IP.Equal() returns always true.
			// To avoid comparing nil to nil, confirm c.IPvX is not nil.
			if (c.IPv4 != nil && c.IPv4.Equal(podIPv4)) || (c.IPv6 != nil && c.IPv6.Equal(podIPv6)) {
				netNsPath, err = getNetNsPath(c.HostVethName)
				if err != nil {
					return err
				}
			}
		} else {
			// We need to bind PodNetConf with Pod. When Coil is a primary CNI IP address can be used for that.
			// However different CNIs manage IP addresses differently, therefore, in egress-only mode instead of
			// storing container's ID in ContainerId, we store pod's UID there to be able to identify pod that was
			// used to create particular PodNetConf.
			if c.ContainerId == string(pod.UID) {
				netNsPath, err = getNetNsPath(c.HostVethName)
				if err != nil {
					return err
				}
			}
		}
	}

	if len(netNsPath) == 0 {
		return fmt.Errorf("failed to find netNsPath")
	}

	containerNS, err := ns.GetNS(netNsPath)
	if err != nil {
		return fmt.Errorf("failed to open netns path %s: %w", netNsPath, err)
	}
	defer containerNS.Close()

	err = containerNS.Do(func(ns.NetNS) error {
		if hook != nil {
			return hook(podIPv4, podIPv6)
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

type linkData struct {
	LinkNetnsid int `json:"link_netnsid"`
}

type netNsData struct {
	Nsid int    `json:"nsid"`
	Name string `json:"name"`
}

func getNetNsPath(hostVethName string) (string, error) {
	command := exec.Command("ip", "-j", "link", "show", hostVethName)
	out, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %s: %w: %s", command.String(), err, string(out))
	}

	var linkData []linkData
	if err := json.Unmarshal(out, &linkData); err != nil {
		return "", fmt.Errorf("failed to unMarshal linkData %w", err)
	}

	if len(linkData) == 0 {
		return "", fmt.Errorf("failed to get link data for %s", hostVethName)
	}

	command = exec.Command("ip", "-j", "netns", "list-id")
	out, err = command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %s: %w: %s", command.String(), err, string(out))
	}

	var netNsData []netNsData
	if err := json.Unmarshal(out, &netNsData); err != nil {
		return "", fmt.Errorf("failed to unMarshal netNsData %w", err)
	}

	for _, netNs := range netNsData {
		if netNs.Nsid == linkData[0].LinkNetnsid {
			return fmt.Sprintf("%s/%s", getNsRunDir(), netNs.Name), nil
		}
	}

	return "", fmt.Errorf("failed to get peer NetNS path for %s", hostVethName)
}

// Reference https://github.com/containernetworking/plugins/blob/509d645ee9ccfee0ad90fe29de3133d0598b7305/pkg/testutils/netns_linux.go#L31-L47
func getNsRunDir() string {
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")

	/// If XDG_RUNTIME_DIR is set, check if the current user owns /var/run.  If
	// the owner is different, we are most likely running in a user namespace.
	// In that case use $XDG_RUNTIME_DIR/netns as runtime dir.
	if xdgRuntimeDir != "" {
		if s, err := os.Stat("/var/run"); err == nil {
			st, ok := s.Sys().(*syscall.Stat_t)
			if ok && int(st.Uid) != os.Geteuid() {
				return path.Join(xdgRuntimeDir, "netns")
			}
		}
	}

	return "/var/run/netns"
}

func (pn *podNetwork) Check(containerId, iface string) error {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	_, err := lookup(containerId, iface)
	if err != nil {
		return err
	}

	// TODO should check further details

	return nil
}

func (pn *podNetwork) Destroy(containerId, iface string) error {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	l, err := lookup(containerId, iface)
	if err == errNotFound {
		return nil
	}
	if err != nil {
		return err
	}

	if err := netlink.LinkDel(l); err != nil {
		return fmt.Errorf("netlink: failed to delete link: %w", err)
	}
	return nil
}

func (pn *podNetwork) List() ([]*PodNetConf, error) {
	pn.mu.Lock()
	defer pn.mu.Unlock()

	return pn.list()
}

func (pn *podNetwork) list() ([]*PodNetConf, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("netlink: failed to list links: %w", err)
	}

	v4Routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: pn.podTableId}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return nil, fmt.Errorf("netlink: failed to list IPv4 routes in table %d: %w", pn.podTableId, err)
	}
	v4Map := make(map[int]net.IP)
	for _, r := range v4Routes {
		v4Map[r.LinkIndex] = r.Dst.IP.To4()
	}

	// TODO: remove this when releasing Coil 2.1
	if pn.registerFromMain {
		v4Routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
		if err != nil {
			return nil, fmt.Errorf("netlink: failed to list IPv4 routes: %w", err)
		}
		for _, r := range v4Routes {
			if r.Protocol != pn.protocolId && r.Protocol != 3 {
				// Calico replaces protocol ID to 3 (== boot)
				continue
			}
			if _, ok := v4Map[r.LinkIndex]; ok {
				continue
			}
			v4Map[r.LinkIndex] = r.Dst.IP.To4()
		}
	}

	v6Routes, err := netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: pn.podTableId}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return nil, fmt.Errorf("netlink: failed to list IPv6 routes in table %d: %w", pn.podTableId, err)
	}
	v6Map := make(map[int]net.IP)
	for _, r := range v6Routes {
		v6Map[r.LinkIndex] = r.Dst.IP.To16()
	}

	// TODO: remove this when releasing Coil 2.1
	if pn.registerFromMain {
		v6Routes, err := netlink.RouteList(nil, netlink.FAMILY_V6)
		if err != nil {
			return nil, fmt.Errorf("netlink: failed to list IPv6 routes: %w", err)
		}
		for _, r := range v6Routes {
			if r.Protocol != pn.protocolId && r.Protocol != 3 {
				continue
			}
			if _, ok := v6Map[r.LinkIndex]; ok {
				continue
			}
			v6Map[r.LinkIndex] = r.Dst.IP.To16()
		}
	}

	var confs []*PodNetConf
	for _, l := range links {
		conf := parseLink(l)
		if conf != nil {
			idx := l.Attrs().Index
			conf.IPv4 = v4Map[idx]
			conf.IPv6 = v6Map[idx]
			conf.HostVethName = l.Attrs().Name
			confs = append(confs, conf)
		}
	}

	return confs, nil
}
