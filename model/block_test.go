package model

import (
	"context"
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func testGetMyBlocks(t *testing.T) {
	t.Parallel()

	m := NewTestEtcdModel(t)

	_, subnet1, _ := net.ParseCIDR("10.1.2.0/24")
	_, subnet2, _ := net.ParseCIDR("10.1.3.0/24")
	_, subnet3, _ := net.ParseCIDR("10.1.4.0/24")

	_, err := m.etcd.Put(context.Background(), blockKey("default", subnet1),
		`{"nodes": {
            "node1": ["10.1.2.16/28", "10.1.2.32/28"]
         }}`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.etcd.Put(context.Background(), blockKey("default", subnet2),
		`{"nodes": {
            "node1": ["10.1.3.32/28"],
            "node2": ["10.1.3.0/28"]
         }}`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.etcd.Put(context.Background(), blockKey("global", subnet3),
		`{"nodes": {
            "node1": ["10.1.4.0/28"],
            "node2": ["10.1.4.16/28"],
            "node3": ["10.1.4.32/28"]
         }}`)
	if err != nil {
		t.Fatal(err)
	}

	myBlocks, err := m.GetMyBlocks(context.Background(), "node1")
	if err != nil {
		t.Fatal(err)
	}
	_, node1block1, _ := net.ParseCIDR("10.1.2.16/28")
	_, node1block2, _ := net.ParseCIDR("10.1.2.32/28")
	_, node1block3, _ := net.ParseCIDR("10.1.3.32/28")
	_, node1block4, _ := net.ParseCIDR("10.1.4.0/28")
	expected := map[string][]*net.IPNet{
		"default": {node1block1, node1block2, node1block3},
		"global":  {node1block4},
	}
	if !cmp.Equal(myBlocks, expected) {
		t.Errorf("not equal: actual=%#v, expected=%#v", myBlocks, expected)
	}

	myBlocks, err = m.GetMyBlocks(context.Background(), "node2")
	if err != nil {
		t.Fatal(err)
	}
	_, node2block1, _ := net.ParseCIDR("10.1.3.0/28")
	_, node2block2, _ := net.ParseCIDR("10.1.4.16/28")
	expected = map[string][]*net.IPNet{
		"default": {node2block1},
		"global":  {node2block2},
	}
	if !cmp.Equal(myBlocks, expected) {
		t.Errorf("not equal: actual=%#v, expected=%#v", myBlocks, expected)
	}

	myBlocks, err = m.GetMyBlocks(context.Background(), "node3")
	if err != nil {
		t.Fatal(err)
	}
	_, node3block1, _ := net.ParseCIDR("10.1.4.32/28")
	expected = map[string][]*net.IPNet{
		"global": {node3block1},
	}
	if !cmp.Equal(myBlocks, expected) {
		t.Errorf("not equal: actual=%#v, expected=%#v", myBlocks, expected)
	}

	myBlocks, err = m.GetMyBlocks(context.Background(), "no-node")
	if err != nil {
		t.Fatal(err)
	}
	if len(myBlocks) != 0 {
		t.Error("no-node should not have any blocks")
	}
}

func testAcquireBlock(t *testing.T) {
	t.Parallel()

	m := NewTestEtcdModel(t)

	_, subnet, _ := net.ParseCIDR("10.11.0.0/16")
	err := m.AddPool(context.Background(), "default", subnet, 5)
	if err != nil {
		t.Fatal(err)
	}

	ipnet1, err := m.AcquireBlock(context.Background(), "node1", "default")
	if err != nil {
		t.Fatal(err)
	}
	if ipnet1.String() != "10.11.0.0/27" {
		t.Errorf("ipnet1.String() != \"10.11.0.0/27\": %v", ipnet1.String())
	}
	ipnet2, err := m.AcquireBlock(context.Background(), "node1", "default")
	if err != nil {
		t.Fatal(err)
	}
	if ipnet2.String() != "10.11.0.32/27" {
		t.Errorf("ipnet2.String() != \"10.11.0.32/27\": %v", ipnet2.String())
	}
	ipnet3, err := m.AcquireBlock(context.Background(), "node2", "default")
	if err != nil {
		t.Fatal(err)
	}
	if ipnet3.String() != "10.11.0.64/27" {
		t.Errorf("ipnet3.String() != \"10.11.0.64/27\": %v", ipnet3.String())
	}
}

func testReleaseBlock(t *testing.T) {
	t.Parallel()

	m := NewTestEtcdModel(t)

	_, subnet, _ := net.ParseCIDR("10.11.0.0/16")
	err := m.AddPool(context.Background(), "default", subnet, 5)
	if err != nil {
		t.Fatal(err)
	}

	ipnet1, err := m.AcquireBlock(context.Background(), "node1", "default")
	if err != nil {
		t.Fatal(err)
	}

	err = m.ReleaseBlock(context.Background(), "node1", "default", ipnet1, true)
	if err != nil {
		t.Error(err)
	}

	err = m.ReleaseBlock(context.Background(), "node1", "default", ipnet1, true)
	if err != ErrNotFound {
		t.Error("double release should return ErrNotFound")
	}

	ipnet2, err := m.AcquireBlock(context.Background(), "node2", "default")
	if err != nil {
		t.Fatal(err)
	}

	err = m.ReleaseBlock(context.Background(), "node1", "default", ipnet2, true)
	if err != ErrNotFound {
		t.Error("double release should return ErrNotFound")
	}
}

func TestBlock(t *testing.T) {
	t.Run("GetMyBlocks", testGetMyBlocks)
	t.Run("AcquireBlock", testAcquireBlock)
	t.Run("ReleaseBlock", testReleaseBlock)
}
