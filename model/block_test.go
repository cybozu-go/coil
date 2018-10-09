package model

import (
	"context"
	"testing"
)

func testAssignBlock(t *testing.T) {
	t.Parallel()

	m := newModel(t)

	pool, err := makeAddressPool("10.11.0.0/16")
	if err != nil {
		t.Fatal(err)
	}

	err = m.AddPool(context.Background(), "default", pool)
	if err != nil {
		t.Fatal(err)
	}

	ipnet1, err := m.AssignBlock(context.Background(), "node1", "default")
	if err != nil {
		t.Fatal(err)
	}
	if ipnet1.String() != "10.11.0.0/27" {
		t.Errorf("ipnet1.String() != \"10.11.0.0/27\": %v", ipnet1.String())
	}
	ipnet2, err := m.AssignBlock(context.Background(), "node1", "default")
	if err != nil {
		t.Fatal(err)
	}
	if ipnet2.String() != "10.11.0.32/27" {
		t.Errorf("ipnet2.String() != \"10.11.0.32/27\": %v", ipnet2.String())
	}
	ipnet3, err := m.AssignBlock(context.Background(), "node2", "default")
	if err != nil {
		t.Fatal(err)
	}
	if ipnet3.String() != "10.11.0.64/27" {
		t.Errorf("ipnet3.String() != \"10.11.0.64/27\": %v", ipnet3.String())
	}
}

func TestBlock(t *testing.T) {
	t.Run("AssignBlock", testAssignBlock)
}
