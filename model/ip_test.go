package model

import (
	"context"
	"net"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func testAllocateIP(t *testing.T) {
	t.Parallel()
	m := newModel(t)

	_, block, err := net.ParseCIDR("10.11.0.0/30")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		_, err := m.AllocateIP(context.Background(), block, "container-"+strconv.Itoa(i))
		if err != nil {
			t.Error(err)
		}
	}
	ips, err := m.GetAllocatedIPs(context.Background(), block)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]net.IP{
		"container-0": net.ParseIP("10.11.0.0"),
		"container-1": net.ParseIP("10.11.0.1"),
		"container-2": net.ParseIP("10.11.0.2"),
		"container-3": net.ParseIP("10.11.0.3"),
	}
	if !cmp.Equal(ips, expected) {
		t.Errorf("!cmd.Equal(ips, expected): %+v, %+v", ips, expected)
	}

	_, err = m.AllocateIP(context.Background(), block, "container-x")
	if err != ErrBlockIsFull {
		t.Error(err)
	}

	ip := net.ParseIP("10.11.0.1")
	err = m.FreeIP(context.Background(), block, ip)
	if err != nil {
		t.Error(err)
	}
	_, err = m.AllocateIP(context.Background(), block, "container-y")
	if err != nil {
		t.Error(err)
	}
}

func TestIP(t *testing.T) {
	t.Run("AllocateIP", testAllocateIP)
}
