package model

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/cybozu-go/coil"
)

func testAddPool(t *testing.T) {
	t.Parallel()
	m := newModel(t)
	pool1, err := makeAddressPool("10.11.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	pool2, err := makeAddressPool("10.12.0.0/16")
	if err != nil {
		t.Fatal(err)
	}

	err = m.AddPool(context.Background(), "default", pool1)
	if err != nil {
		t.Fatal(err)
	}

	err = m.AddPool(context.Background(), "default", pool2)
	if err != ErrPoolExists {
		t.Fatal(errors.New("duplicate operation should be error"))
	}

	err = m.AddPool(context.Background(), "another", pool1)
	if err != ErrUsedSubnet {
		t.Fatal(errors.New("should be error: subnet already in use"))
	}

	err = m.AddPool(context.Background(), "another", pool2)
	if err != nil {
		t.Fatal(err)
	}
}

func makeAddressPool(subnets ...string) (*coil.AddressPool, error) {
	p := new(coil.AddressPool)
	p.BlockSize = 5
	for _, s := range subnets {
		_, ipNet, err := net.ParseCIDR(s)
		if err != nil {
			return nil, err
		}
		p.Subnets = append(p.Subnets, ipNet)
	}
	return p, nil
}

func TestPool(t *testing.T) {
	t.Run("AddPool", testAddPool)
}
