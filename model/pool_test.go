package model

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/cybozu-go/coil"
)

func testAddPool(t *testing.T) {
	t.Parallel()
	m := NewTestEtcdModel(t)

	_, subnet1, _ := net.ParseCIDR("10.11.0.0/16")
	_, subnet2, _ := net.ParseCIDR("10.12.0.0/16")

	err := m.AddPool(context.Background(), "!invalid name", subnet1, 5)
	if err == nil {
		t.Fatal("pool name must be validated")
	}
	err = m.AddPool(context.Background(), "default", subnet1, 17)
	if err == nil {
		t.Fatal("pool must be validated")
	}

	err = m.AddPool(context.Background(), "default", subnet1, 5)
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
	if len(pool.Subnets) != 1 || pool.Subnets[0].String() != subnet1.String() {
		t.Error("wrong subnet:", pool.Subnets)
	}
	if pool.BlockSize != 5 {
		t.Error("wrong block size:", pool.BlockSize)
	}

	err = m.AddPool(context.Background(), "default", subnet2, 5)
	if err != ErrPoolExists {
		t.Fatal("duplicate operation should be error")
	}

	err = m.AddPool(context.Background(), "another", subnet1, 5)
	if err != ErrUsedSubnet {
		t.Fatal("should be error: subnet already in use")
	}

	err = m.AddPool(context.Background(), "another", subnet2, 5)
	if err != nil {
		t.Fatal(err)
	}
}

func testAddSubnet(t *testing.T) {
	t.Parallel()
	m := NewTestEtcdModel(t)

	_, subnet1, _ := net.ParseCIDR("10.12.0.0/24")
	_, subnet2, _ := net.ParseCIDR("10.12.1.0/24")
	_, subnet3, _ := net.ParseCIDR("10.12.2.0/30")

	err := m.AddSubnet(context.Background(), "default", subnet1)
	if err != ErrNotFound {
		t.Error(err)
	}

	_, poolSubnet, _ := net.ParseCIDR("10.11.0.0/16")
	err = m.AddPool(context.Background(), "default", poolSubnet, 5)
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
	m := NewTestEtcdModel(t)

	err := m.RemovePool(context.Background(), "default")
	if err != ErrNotFound {
		t.Error(err)
	}

	_, subnet1, _ := net.ParseCIDR("10.11.0.0/16")
	err = m.AddPool(context.Background(), "default", subnet1, 5)
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

func TestPool(t *testing.T) {
	t.Run("AddPool", testAddPool)
	t.Run("AddSubnet", testAddSubnet)
	t.Run("RemovePool", testRemovePool)
}
