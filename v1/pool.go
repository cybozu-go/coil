package coil

import (
	"encoding/json"
	"errors"
	"net"
)

const (
	maxLog2Blocks = 14
)

// AddressPool is a collection of subnets
type AddressPool struct {
	Subnets   []*net.IPNet
	BlockSize int
}

// Validate validates pool.
func (p AddressPool) Validate() error {
	if len(p.Subnets) == 0 {
		return errors.New("no subnets")
	}

	for _, n := range p.Subnets {
		ones, bits := n.Mask.Size()
		log2blocks := bits - ones - p.BlockSize
		if log2blocks > maxLog2Blocks {
			return errors.New("too small block size")
		}
		if log2blocks < 0 {
			return errors.New("too large block size")
		}
	}

	return nil
}

// MarshalJSON implements json.Marshaler
func (p AddressPool) MarshalJSON() ([]byte, error) {
	t := struct {
		Subnets   []string `json:"subnets"`
		BlockSize int      `json:"block_size"`
	}{}
	for _, n := range p.Subnets {
		t.Subnets = append(t.Subnets, n.String())
	}
	t.BlockSize = p.BlockSize
	return json.Marshal(t)
}

// UnmarshalJSON implements json.Unmarshaler
func (p *AddressPool) UnmarshalJSON(data []byte) error {
	t := struct {
		Subnets   []string `json:"subnets"`
		BlockSize int      `json:"block_size"`
	}{}
	err := json.Unmarshal(data, &t)
	if err != nil {
		return err
	}
	for _, n := range t.Subnets {
		_, ipNet, err := net.ParseCIDR(n)
		if err != nil {
			return err
		}
		p.Subnets = append(p.Subnets, ipNet)
	}
	p.BlockSize = t.BlockSize
	return nil
}
