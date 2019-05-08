package model

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/cybozu-go/coil"
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

	assignment := coil.IPAssignment{
		ContainerID: containerID,
		Namespace:   "default",
		Pod:         "test",
		CreatedAt:   time.Now().UTC(),
	}
	_, err = m.AllocateIP(context.Background(), block, assignment)
	if err != nil {
		t.Fatal(err)
	}

	info, _, err := m.GetAddressInfo(context.Background(), net.ParseIP("10.11.0.0"))
	if err != nil {
		t.Fatal(err)
	}

	if *info != assignment {
		t.Errorf("expected info: %v, actual: %v", assignment, *info)
	}

	_, _, err = m.GetAddressInfo(context.Background(), net.ParseIP("10.11.0.1"))
	if err != ErrNotFound {
		t.Errorf("expected error: ErrNotFound, actual: %v", err)
	}
}

func TestAddress(t *testing.T) {
	t.Run("GetAddressInfo", testGetAddressInfo)
}
