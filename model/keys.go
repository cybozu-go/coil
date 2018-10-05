package model

import (
	"fmt"
	"net"
)

const (
	keyPool   = "pool/"
	keySubnet = "subnet/"
	keyIP     = "ip/"
	keyBlock  = "block/"
)

func poolKey(name string) string {
	return keyPool + name
}

func subnetKey(n *net.IPNet) string {
	return keySubnet + n.IP.String()
}

func ipKey(block *net.IPNet, offset int) string {
	return fmt.Sprintf("%s%s/%d", keyIP, block.IP.String(), offset)
}

func blockKey(pool string, subnet *net.IPNet) string {
	return fmt.Sprintf("%s%s/%s", keyBlock, pool, subnet.IP.String())
}
