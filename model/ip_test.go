package model

import (
	"context"
	"net"
	"testing"
)

func testAllocateIP(t *testing.T) {
	t.Parallel()
	m := newModel(t)

	_, block, err := net.ParseCIDR("10.11.0.0/30")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		_, err := m.AllocateIP(context.Background(), block, "1")
		if err != nil {
			t.Error(err)
		}
	}
	_, err = m.AllocateIP(context.Background(), block, "1")
	if err != ErrBlockIsFull {
		t.Error(err)
	}

	ip := net.ParseIP("10.11.0.1")
	err = m.FreeIP(context.Background(), block, ip)
	if err != nil {
		t.Error(err)
	}
	_, err = m.AllocateIP(context.Background(), block, "1")
	if err != nil {
		t.Error(err)
	}
}

func TestIP(t *testing.T) {
	t.Run("AllocateIP", testAllocateIP)
}
