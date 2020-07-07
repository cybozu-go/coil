package coild

import (
	"net"

	"github.com/vishvananda/netlink"
)

// syncRoutingTable synchronizes a set of address blocks with a kernel routing table.
func syncRoutingTable(tableID, protocol int, blocks []*net.IPNet) error {
	h, err := netlink.NewHandle()
	if err != nil {
		return err
	}
	defer h.Delete()

	filter := &netlink.Route{Table: tableID}
	routes, err := h.RouteListFiltered(0, filter, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}

	newRouteMap := make(map[string]bool)
	for _, b := range blocks {
		newRouteMap[b.String()] = true
	}

	curRouteMap := make(map[string]bool)
	for _, r := range routes {
		rKey := r.Dst.String()
		curRouteMap[rKey] = true
		if !newRouteMap[rKey] {
			err := h.RouteDel(&r)
			if err != nil {
				return err
			}
		}
	}

	lo, err := h.LinkByName("lo")
	if err != nil {
		return err
	}

	for _, subnet := range blocks {
		rKey := subnet.String()
		if curRouteMap[rKey] {
			continue
		}

		r := &netlink.Route{
			Scope:     netlink.SCOPE_UNIVERSE,
			Dst:       subnet,
			Table:     tableID,
			LinkIndex: lo.Attrs().Index,
			Protocol:  protocol,
		}
		err := h.RouteAdd(r)
		if err != nil {
			return err
		}
	}

	return nil
}

// addBlockRouting adds an address block to a kernel routing table.
func addBlockRouting(tableID, protocol int, block *net.IPNet) error {
	h, err := netlink.NewHandle()
	if err != nil {
		return err
	}
	defer h.Delete()

	lo, err := h.LinkByName("lo")
	if err != nil {
		return err
	}

	r := &netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst:       block,
		Table:     tableID,
		LinkIndex: lo.Attrs().Index,
		Protocol:  protocol,
	}
	return h.RouteAdd(r)
}

// deleteBlockRouting deletes an address block from a kernel routing table.
func deleteBlockRouting(tableID, protocol int, block *net.IPNet) error {
	h, err := netlink.NewHandle()
	if err != nil {
		return err
	}
	defer h.Delete()

	lo, err := h.LinkByName("lo")
	if err != nil {
		return err
	}

	r := &netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst:       block,
		Table:     tableID,
		LinkIndex: lo.Attrs().Index,
		Protocol:  protocol,
	}
	return h.RouteDel(r)
}
