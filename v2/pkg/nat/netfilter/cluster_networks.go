package netfilter

import (
	"fmt"
	"net"
)

type ClusterNetworks struct {
	v4 []*net.IPNet
	v6 []*net.IPNet
}

func (cn *ClusterNetworks) IsEmpty() bool {
	return len(cn.v4) == 0 && len(cn.v6) == 0
}

// ParseClusterNetworks parses --cluster-networks CIDR strings into net.IPNet values.
func ParseClusterNetworks(cidrs []string) (*ClusterNetworks, error) {
	var v4, v6 []*net.IPNet
	for _, s := range cidrs {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", s, err)
		}
		if n.IP.To4() != nil {
			v4 = append(v4, n)
		} else {
			v6 = append(v6, n)
		}
	}
	return &ClusterNetworks{
		v4: v4,
		v6: v6,
	}, nil
}

// ValidateClusterNetworks checks that clusterNetworks does not exceed
// MaxInClusterNetworks entries per IP family.
func (cn *ClusterNetworks) Validate() error {
	if len(cn.v4) > MaxInClusterNetworks {
		return fmt.Errorf("too many IPv4 cluster networks: %d (max %d)", len(cn.v4), MaxInClusterNetworks)
	}
	if len(cn.v6) > MaxInClusterNetworks {
		return fmt.Errorf("too many IPv6 cluster networks: %d (max %d)", len(cn.v6), MaxInClusterNetworks)
	}
	return nil
}
