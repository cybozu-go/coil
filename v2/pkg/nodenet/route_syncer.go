package nodenet

import (
	"fmt"
	"net"
	"sync"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
)

// GatewayInfo is a set of destination networks for a gateway.
type GatewayInfo struct {
	Gateway  net.IP
	Networks []*net.IPNet
}

// RouteSyncer is the interface to program direct routing.
type RouteSyncer interface {
	// Sync synchronizes the kernel routing table with the given routes.
	Sync([]GatewayInfo) error
}

// NewRouteSyncer creates a DirectRouter that marks routes with protocolId.
//
// protocolId must be different from the ID for NewPodNetwork.
func NewRouteSyncer(protocolId int, log logr.Logger) RouteSyncer {
	return &routeSyncer{
		protocolId: netlink.RouteProtocol(protocolId),
		log:        log,
	}
}

type routeSyncer struct {
	protocolId netlink.RouteProtocol
	log        logr.Logger

	mu sync.Mutex
}

func (d *routeSyncer) Sync(gis []GatewayInfo) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.log.Info("synchronizing the main routing table", "gateways", len(gis))
	routes, err := netlink.RouteListFiltered(0, &netlink.Route{Protocol: d.protocolId}, netlink.RT_FILTER_PROTOCOL)
	if err != nil {
		return fmt.Errorf("netlink: failed to list routes: %w", err)
	}

	routeMap := make(map[string]*netlink.Route)
	for _, gi := range gis {
		strIP := gi.Gateway.String()
		for _, n := range gi.Networks {
			routeMap[strIP+" "+n.String()] = &netlink.Route{
				Dst:      n,
				Gw:       gi.Gateway,
				Scope:    netlink.SCOPE_UNIVERSE,
				Protocol: d.protocolId,
			}
		}
	}

	currentMap := make(map[string]bool)
	for _, r := range routes {
		key := r.Gw.String() + " " + r.Dst.String()
		if _, ok := routeMap[key]; !ok {
			if err := netlink.RouteDel(&r); err != nil {
				return fmt.Errorf("netlink: failed to delete route: %w", err)
			}
			continue
		}
		currentMap[key] = true
	}

	for k, v := range routeMap {
		if !currentMap[k] {
			if err := netlink.RouteAdd(v); err != nil {
				return fmt.Errorf("sync: netlink: failed to add route to %s: %w", k, err)
			}
			d.log.Info("added", "dst", v.Dst.String())
		}
	}

	return nil
}
