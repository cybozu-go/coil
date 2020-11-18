package nodenet

import (
	"fmt"

	"github.com/vishvananda/netlink"
)

// DetectMTU returns the _minimum_ MTU value of the physical network links.
// This may return zero if it cannot detect any physical link or encounters an error.
func DetectMTU() (int, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return 0, fmt.Errorf("netlink: failed to list links: %w", err)
	}

	mtu := 0
	for _, link := range links {
		dev, ok := link.(*netlink.Device)
		if !ok {
			//fmt.Println("skipping non device", link.Attrs().Name)
			continue
		}

		if dev.Attrs().OperState != netlink.OperUp {
			//fmt.Println("skipping down link", dev.Name)
			continue
		}

		if dev.MTU == 0 {
			continue
		}

		if mtu == 0 {
			mtu = dev.MTU
			continue
		}

		if dev.MTU < mtu {
			mtu = dev.MTU
		}
	}

	return mtu, nil
}
