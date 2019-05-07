package model

import (
	"context"
	"net"
	"testing"
)

const (
	containerID = "5451faf2-da4f-4690-b024-b77518982f61"
)

func testGetAddressInfo(t *testing.T) {
	t.Parallel()
	m := newModel(t)

	_, block, err := net.ParseCIDR("10.11.0.0/30")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.AllocateIP(context.Background(), block, containerID)
	if err != nil {
		t.Fatal(err)
	}

	info, err := m.GetAddressInfo(context.Background(), net.ParseIP("10.11.0.0"))
	if err != nil {
		t.Fatal(err)
	}

	if info != containerID {
		t.Errorf("expected info: %s, actual: %s", containerID, info)
	}

	_, err = m.GetAddressInfo(context.Background(), net.ParseIP("10.11.0.1"))
	if err != ErrNotFound {
		t.Errorf("expected error: ErrNotFound, actual: %v", err)
	}
}

func TestAddress(t *testing.T) {
	t.Run("GetAddressInfo", testGetAddressInfo)
}
