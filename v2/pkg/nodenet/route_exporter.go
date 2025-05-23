package nodenet

import (
	"fmt"
	"net"
	"sync"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
)

// RouteExporter exports subnets to a Linux kernel routing table.
type RouteExporter interface {
	Sync([]*net.IPNet) error
}

// NewRouteExporter creates a new RouteExporter
func NewRouteExporter(tableId, protocolId int, log logr.Logger) RouteExporter {
	return &routeExporter{
		tableId:    tableId,
		protocolId: netlink.RouteProtocol(protocolId),
		log:        log,
	}
}

type routeExporter struct {
	tableId    int
	protocolId netlink.RouteProtocol
	log        logr.Logger

	mu sync.Mutex
}

func (r *routeExporter) Sync(nets []*net.IPNet) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.log.Info("synchronizing routing table", "table-id", r.tableId)

	h, err := netlink.NewHandle()
	if err != nil {
		r.log.Error(err, "netlink: failed to open handle")
		return fmt.Errorf("netlink: failed to open handle: %w", err)
	}
	defer h.Close()

	lo, err := h.LinkByName("lo")
	if err != nil {
		r.log.Error(err, "netlink: failed to get link lo")
		return fmt.Errorf("netlink: failed to get link lo: %w", err)
	}

	filter := &netlink.Route{Table: r.tableId}
	routes, err := h.RouteListFiltered(0, filter, netlink.RT_FILTER_TABLE)
	if err != nil {
		r.log.Error(err, "netlink: failed to list routes")
		return fmt.Errorf("netlink: failed to list routes: %w", err)
	}
	routeHash := make(map[string]bool)
	for _, r := range routes {
		if r.Dst != nil {
			routeHash[r.Dst.String()] = true
		}
	}

	// add routes
	netHash := make(map[string]bool)
	for _, n := range nets {
		key := n.String()
		netHash[key] = true
		if routeHash[key] {
			continue
		}

		err := h.RouteAdd(&netlink.Route{
			Scope:     netlink.SCOPE_UNIVERSE,
			Dst:       n,
			Table:     r.tableId,
			LinkIndex: lo.Attrs().Index,
			Protocol:  r.protocolId,
		})
		if err != nil {
			r.log.Error(err, "exporter: netlink: failed to add route", "network", key)
			return fmt.Errorf("exporter: netlink: failed to add route to %s: %w", key, err)
		}
	}

	// remove routes
	for _, route := range routes {
		if route.Dst == nil {
			continue
		}
		key := route.Dst.String()
		if netHash[key] {
			continue
		}

		err := h.RouteDel(&route)
		if err != nil {
			r.log.Error(err, "netlink: failed to delete route", "route", key)
			return fmt.Errorf("netlink: failed to delete route to %s: %w", key, err)
		}
	}
	return nil
}
