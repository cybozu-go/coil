package ipam

import (
	"net"
	"testing"
)

func TestAllocator(t *testing.T) {
	t.Run("v4", testAllocatorV4)
	t.Run("v6", testAllocatorV6)
	t.Run("dual", testAllocatorDual)
}

func testAllocatorV4(t *testing.T) {
	t.Parallel()

	ipv4 := "10.2.3.0/30"
	a := newAllocator(&ipv4, nil)
	if !a.isEmpty() {
		t.Error("new allocator should be empty")
	}

	if _, ok := a.register(net.ParseIP("192.168.0.1"), nil); ok {
		t.Error("should ignore out of scope address")
	}

	if idx, ok := a.register(net.ParseIP("10.2.3.0"), nil); !ok {
		t.Error("should register a member address")
	} else if idx != 0 {
		t.Error("idx should be 0, but", idx)
	}

	if idx, ok := a.register(net.ParseIP("10.2.3.2"), nil); !ok {
		t.Error("should register a member address")
	} else if idx != 2 {
		t.Error("idx should be 2, but", idx)
	}

	if a.isEmpty() || a.isFull() {
		t.Error("should be not empty nor full")
	}

	if ip1, ip2, idx, ok := a.allocate(); !ok {
		t.Error("should allocate addresses")
	} else {
		if !ip1.Equal(net.ParseIP("10.2.3.1")) {
			t.Error("unexpected ip1:", ip1)
		}
		if ip2 != nil {
			t.Error("should not allocate IPv6 address")
		}
		if idx != 1 {
			t.Error("idx should be 1, but", idx)
		}
	}

	if _, _, _, ok := a.allocate(); !ok {
		t.Error("should allocate addresses")
	}

	if !a.isFull() {
		t.Error("should be full")
	}

	if _, _, _, ok := a.allocate(); ok {
		t.Error("should not allocate addresses")
	}

	a.free(1)
	a.free(1)

	if a.isFull() {
		t.Error("should not be full")
	}

	if _, _, idx, ok := a.allocate(); !ok {
		t.Error("should allocate addresses")
	} else if idx != 1 {
		t.Error("idx should be 1, but", idx)
	}

	if !a.isFull() {
		t.Error("should be full")
	}
}

func testAllocatorV6(t *testing.T) {
	t.Parallel()

	ipv6 := "fd02::/126"
	a := newAllocator(nil, &ipv6)
	if !a.isEmpty() {
		t.Error("new allocator should be empty")
	}

	if _, ok := a.register(nil, net.ParseIP("fe03::0001")); ok {
		t.Error("should ignore out of scope address")
	}

	if idx, ok := a.register(nil, net.ParseIP("fd02::")); !ok {
		t.Error("should register a member address")
	} else if idx != 0 {
		t.Error("idx should be 0, but", idx)
	}

	if idx, ok := a.register(nil, net.ParseIP("fd02::0002")); !ok {
		t.Error("should register a member address")
	} else if idx != 2 {
		t.Error("idx should be 2, but", idx)
	}

	if a.isEmpty() || a.isFull() {
		t.Error("should be not empty nor full")
	}

	if ip1, ip2, idx, ok := a.allocate(); !ok {
		t.Error("should allocate addresses")
	} else {
		if ip1 != nil {
			t.Error("should not allocate IPv4 address")
		}
		if !ip2.Equal(net.ParseIP("fd02::0001")) {
			t.Error("unexpected ip2:", ip2)
		}
		if idx != 1 {
			t.Error("idx should be 1, but", idx)
		}
	}

	if _, _, _, ok := a.allocate(); !ok {
		t.Error("should allocate addresses")
	}

	if !a.isFull() {
		t.Error("should be full")
	}

	if _, _, _, ok := a.allocate(); ok {
		t.Error("should not allocate addresses")
	}

	a.free(1)
	a.free(1)

	if a.isFull() {
		t.Error("should not be full")
	}

	if _, _, idx, ok := a.allocate(); !ok {
		t.Error("should allocate addresses")
	} else if idx != 1 {
		t.Error("idx should be 1, but", idx)
	}

	if !a.isFull() {
		t.Error("should be full")
	}
}

func testAllocatorDual(t *testing.T) {
	t.Parallel()

	ipv4 := "10.2.3.0/30"
	ipv6 := "fd02::/126"
	a := newAllocator(&ipv4, &ipv6)
	if !a.isEmpty() {
		t.Error("new allocator should be empty")
	}

	if _, ok := a.register(net.ParseIP("192.168.0.1"), net.ParseIP("fe03::0001")); ok {
		t.Error("should ignore out of scope address")
	}

	if idx, ok := a.register(net.ParseIP("10.2.3.0"), net.ParseIP("fd02::")); !ok {
		t.Error("should register a member address")
	} else if idx != 0 {
		t.Error("idx should be 0, but", idx)
	}

	if idx, ok := a.register(net.ParseIP("10.2.3.2"), net.ParseIP("fd02::0002")); !ok {
		t.Error("should register a member address")
	} else if idx != 2 {
		t.Error("idx should be 2, but", idx)
	}

	if a.isEmpty() || a.isFull() {
		t.Error("should be not empty nor full")
	}

	if ip1, ip2, idx, ok := a.allocate(); !ok {
		t.Error("should allocate addresses")
	} else {
		if !ip1.Equal(net.ParseIP("10.2.3.1")) {
			t.Error("unexpected ip1:", ip1)
		}
		if !ip2.Equal(net.ParseIP("fd02::0001")) {
			t.Error("unexpected ip2:", ip2)
		}
		if idx != 1 {
			t.Error("idx should be 1, but", idx)
		}
	}

	if _, _, _, ok := a.allocate(); !ok {
		t.Error("should allocate addresses")
	}

	if !a.isFull() {
		t.Error("should be full")
	}

	if _, _, _, ok := a.allocate(); ok {
		t.Error("should not allocate addresses")
	}

	a.free(1)
	a.free(1)

	if a.isFull() {
		t.Error("should not be full")
	}

	if _, _, idx, ok := a.allocate(); !ok {
		t.Error("should allocate addresses")
	} else if idx != 1 {
		t.Error("idx should be 1, but", idx)
	}

	if !a.isFull() {
		t.Error("should be full")
	}
}
