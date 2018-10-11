package main

import (
	"fmt"
	"log"
	"net"

	"github.com/vishvananda/netlink"
)

func addLink() (netlink.Link, error) {
	dummy := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name:  "node0",
			Flags: net.FlagUp,
			MTU:   1500,
		},
	}

	err := netlink.LinkAdd(dummy)
	if err != nil {
		return nil, err
	}
	return dummy, nil
}

func addRoutes() error {
	h, err := netlink.NewHandle()
	if err != nil {
		return err
	}
	defer h.Delete()

	dev, err := addLink()
	if err != nil {
		return err
	}

	_, subnet, _ := net.ParseCIDR("10.11.22.33/32")
	r := &netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst:       subnet,
		Table:     119,
		LinkIndex: dev.Attrs().Index,
	}
	return h.RouteAdd(r)
}

func listRoutes() error {
	h, err := netlink.NewHandle()
	if err != nil {
		return err
	}
	defer h.Delete()

	routes, err := h.RouteListFiltered(0, &netlink.Route{
		Table: 119,
	}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}
	for _, r := range routes {
		fmt.Println(r.String())
	}
	return nil
}

func main() {
	err := addRoutes()
	if err != nil {
		log.Fatal(err)
	}
	err = listRoutes()
	if err != nil {
		log.Fatal(err)
	}
}
