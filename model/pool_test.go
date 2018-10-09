package model

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/cybozu-go/coil"
	"github.com/google/go-cmp/cmp"
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

	resp, err := m.etcd.Get(context.Background(), poolKey("default"))
	if err != nil {
		t.Fatal(err)
	}

	pool := new(coil.AddressPool)
	err = json.Unmarshal(resp.Kvs[0].Value, pool)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(pool, pool1) {
		t.Fatalf("pool != poo1; %v != %v", pool, pool1)
	}

	err = m.AddPool(context.Background(), "default", pool2)
	if err != ErrPoolExists {
		t.Fatal("duplicate operation should be error")
	}

	err = m.AddPool(context.Background(), "another", pool1)
	if err != ErrUsedSubnet {
		t.Fatal("should be error: subnet already in use")
	}

	err = m.AddPool(context.Background(), "another", pool2)
	if err != nil {
		t.Fatal(err)
	}
}

func testAddSubnet(t *testing.T) {
	t.Parallel()
	m := newModel(t)
	pool, err := makeAddressPool("10.11.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	_, subnet1, _ := net.ParseCIDR("10.12.0.0/24")
	_, subnet2, _ := net.ParseCIDR("10.12.1.0/24")
	_, subnet3, _ := net.ParseCIDR("10.12.2.0/30")

	err = m.AddSubnet(context.Background(), "default", subnet1)
	if err != ErrNotFound {
		t.Error(err)
	}

	err = m.AddPool(context.Background(), "default", pool)
	if err != nil {
		t.Fatal(err)
	}

	err = m.AddSubnet(context.Background(), "default", subnet1)
	if err != nil {
		t.Error(err)
	}

	err = m.AddSubnet(context.Background(), "default", subnet1)
	if err != ErrUsedSubnet {
		t.Error(err)
	}

	err = m.AddSubnet(context.Background(), "default", subnet2)
	if err != nil {
		t.Error(err)
	}

	err = m.AddSubnet(context.Background(), "default", subnet3)
	if err == nil {
		t.Error("should be validated")
	}
}

func testRemovePool(t *testing.T) {
	t.Parallel()
	m := newModel(t)
	pool, err := makeAddressPool("10.11.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	_, subnet1, _ := net.ParseCIDR("10.11.0.0/16")

	err = m.RemovePool(context.Background(), "default")
	if err != ErrNotFound {
		t.Error(err)
	}

	err = m.AddPool(context.Background(), "default", pool)
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.etcd.Put(context.Background(), blockKey("default", subnet1), "")
	if err != nil {
		t.Fatal(err)
	}

	err = m.RemovePool(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := m.etcd.Get(context.Background(), subnetKey(subnet1))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count != 0 {
		t.Error("resp.Count should be 0")
	}

	resp, err = m.etcd.Get(context.Background(), blockKey("default", subnet1))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count != 0 {
		t.Error("resp.Count should be 0")
	}

	err = m.RemovePool(context.Background(), "default")
	if err != ErrNotFound {
		t.Error(err)
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
	t.Run("AddSubnet", testAddSubnet)
	t.Run("RemovePool", testRemovePool)
}
