package model

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/netutil"
	"github.com/google/go-cmp/cmp"
)

func testAllocateIP(t *testing.T) {
	t.Parallel()
	m := NewTestEtcdModel(t)

	_, subnet, err := net.ParseCIDR("10.11.0.0/28")
	if err != nil {
		t.Fatal(err)
	}

	err = m.AddPool(context.Background(), "default", subnet, 2)
	if err != nil {
		t.Fatal(err)
	}

	block, err := m.AcquireBlock(context.Background(), "node1", "default")
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
		"container-0": block.IP,
		"container-1": netutil.IntToIP4(netutil.IP4ToInt(block.IP) + 1),
		"container-2": netutil.IntToIP4(netutil.IP4ToInt(block.IP) + 2),
		"container-3": netutil.IntToIP4(netutil.IP4ToInt(block.IP) + 3),
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

	ip := expected["container-1"]
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
