package main

import (
	"log"
	"net"

	"github.com/vishvananda/netlink"
)

func addRoutes() error {
	h, err := netlink.NewHandle()
	if err != nil {
		return err
	}
	defer h.Delete()

	_, subnet, _ := net.ParseCIDR("10.11.22.33/32")
	r := &netlink.Route{
		Scope: netlink.SCOPE_UNIVERSE,
		Dst:   subnet,
		Table: 119,
		Gw:    net.ParseIP("10.11.22.1"),
	}
	return h.RouteAdd(r)
}

func main() {
	err := addRoutes()
	if err != nil {
		log.Fatal(err)
	}
}
