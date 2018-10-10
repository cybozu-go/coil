package coil

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/cybozu-go/netutil"
)

// ErrBlockNotFound is returned when a target block does not exist.
var ErrBlockNotFound = errors.New("block not found")

// BlockAssignment holds address block assignment information for a subnet
type BlockAssignment struct {
	FreeList []*net.IPNet            `json:"free"`
	Nodes    map[string][]*net.IPNet `json:"nodes"`
}

// EmptyAssignment returns an empty block assignment for ipnet and blockSize.
func EmptyAssignment(ipnet *net.IPNet, blockSize int) BlockAssignment {
	ones, bits := ipnet.Mask.Size()
	freeCount := 1 << uint((bits-ones)-blockSize)
	blockLength := 1 << uint(blockSize)

	var v BlockAssignment
	v.Nodes = make(map[string][]*net.IPNet)
	v.FreeList = make([]*net.IPNet, freeCount)

	base := netutil.IP4ToInt(ipnet.IP)
	mask := net.CIDRMask(bits-blockSize, bits)
	for i := 0; i < freeCount; i++ {
		ip := netutil.IntToIP4(base + uint32(blockLength*i))
		v.FreeList[i] = &net.IPNet{IP: ip, Mask: mask}
	}
	return v
}

// MarshalJSON implements Marshaler
func (ba BlockAssignment) MarshalJSON() ([]byte, error) {
	t := struct {
		FreeList []string            `json:"free"`
		Nodes    map[string][]string `json:"nodes"`
	}{}
	t.Nodes = make(map[string][]string)
	for _, ipNet := range ba.FreeList {
		t.FreeList = append(t.FreeList, ipNet.String())
	}
	for node, ipNets := range ba.Nodes {
		for _, ipNet := range ipNets {
			t.Nodes[node] = append(t.Nodes[node], ipNet.String())
		}
	}
	return json.Marshal(t)
}

// UnmarshalJSON implements Unmarshaler
func (ba *BlockAssignment) UnmarshalJSON(data []byte) error {
	ba.Nodes = make(map[string][]*net.IPNet)
	t := struct {
		FreeList []string            `json:"free"`
		Nodes    map[string][]string `json:"nodes"`
	}{}
	err := json.Unmarshal(data, &t)
	if err != nil {
		return err
	}
	for _, n := range t.FreeList {
		_, ipNet, err := net.ParseCIDR(n)
		if err != nil {
			return err
		}
		ba.FreeList = append(ba.FreeList, ipNet)
	}
	for node, ipNets := range t.Nodes {
		for _, ipNetStr := range ipNets {
			_, ipNet, err := net.ParseCIDR(ipNetStr)
			if err != nil {
				return err
			}
			ba.Nodes[node] = append(ba.Nodes[node], ipNet)
		}
	}
	return nil
}

// FindBlock returns index of target block
func (ba *BlockAssignment) FindBlock(node string, block *net.IPNet) int {
	for idx, b := range ba.Nodes[node] {
		if b.String() == block.String() {
			return idx
		}
	}
	return -1
}

// ReleaseBlock move target block to freeList from target node
func (ba *BlockAssignment) ReleaseBlock(node string, block *net.IPNet) error {
	idx := ba.FindBlock(node, block)
	if idx == -1 {
		return ErrBlockNotFound
	}

	ba.FreeList = append(ba.FreeList, block)

	blist := ba.Nodes[node]

	if len(blist) == 1 {
		delete(ba.Nodes, node)
		return nil
	}

	newList := blist[0 : len(blist)-1]
	copy(newList[idx:], blist[idx+1:])
	ba.Nodes[node] = newList
	return nil
}
