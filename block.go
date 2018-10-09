package coil

import (
	"encoding/json"
	"errors"
	"net"
)

// BlockAssignment holds address block assignment information for a subnet
type BlockAssignment struct {
	FreeList []*net.IPNet            `json:"free"`
	Nodes    map[string][]*net.IPNet `json:"nodes"`
}

// ErrBlockNotFound is returned when a target block does not exist.
var ErrBlockNotFound = errors.New("block not found")

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
	ba.FreeList = append(ba.FreeList, block)

	idx := ba.FindBlock(node, block)
	if idx == -1 {
		return ErrBlockNotFound
	}
	newBlocks := make([]*net.IPNet, idx, len(ba.Nodes[node])-1)
	copy(newBlocks, ba.Nodes[node][0:idx])
	newBlocks = append(newBlocks, ba.Nodes[node][idx+1:]...)

	if len(newBlocks) == 0 {
		delete(ba.Nodes, node)
	} else {
		ba.Nodes[node] = newBlocks
	}

	return nil
}
