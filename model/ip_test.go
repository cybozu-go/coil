package model

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/cybozu-go/coil"
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
		_, err := m.AllocateIP(context.Background(), block, coil.IPAssignment{
			ContainerID: fmt.Sprintf("container-%d", i),
			Namespace:   "default",
			Pod:         "pod-" + strconv.Itoa(i),
			CreatedAt:   time.Now().UTC(),
		})
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

	_, err = m.AllocateIP(context.Background(), block, coil.IPAssignment{
		ContainerID: "4aed6222-375d-498e-a40a-e653c37cb0d9",
		Namespace:   "default",
		Pod:         "pod-x",
		CreatedAt:   time.Now().UTC(),
	})
	if err != ErrBlockIsFull {
		t.Error(err)
	}

	ip := net.ParseIP("10.11.0.1")
	_, modRev, err := m.GetAddressInfo(context.Background(), ip)
	if err != nil {
		t.Error(err)
	}
	err = m.FreeIP(context.Background(), block, ip, modRev)
	if err != nil {
		t.Error(err)
	}
	err = m.FreeIP(context.Background(), block, ip, modRev)
	if err != ErrModRevDiffers {
		t.Error("ErrModRevDiffers should be returned")
	}
	_, err = m.AllocateIP(context.Background(), block, coil.IPAssignment{
		ContainerID: "0a966833-a244-42fe-8f78-cc5d68f50ad0",
		Namespace:   "default",
		Pod:         "pod-y",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Error(err)
	}
}

func TestIP(t *testing.T) {
	t.Run("AllocateIP", testAllocateIP)
}
