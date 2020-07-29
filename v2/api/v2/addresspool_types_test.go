package v2

import (
	"net"
	"testing"
)

func TestSubnetSet(t *testing.T) {
	t.Run("Validate", testSubnetSetValidate)
	t.Run("Is", testSubnetSetIs)
	t.Run("Equal", testSubnetSetEqual)
	t.Run("GetBlock", testSubnetSetGetBlock)
}

func makeSubnetSet(ipv4, ipv6 string) SubnetSet {
	p4 := &ipv4
	p6 := &ipv6
	if len(ipv4) == 0 {
		p4 = nil
	}
	if len(ipv6) == 0 {
		p6 = nil
	}
	return SubnetSet{IPv4: p4, IPv6: p6}
}

func testSubnetSetValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		minSize   int
		subnetSet SubnetSet
		expectErr bool
	}{
		{"empty", 2, makeSubnetSet("", ""), true},
		{"valid-ipv4", 2, makeSubnetSet("10.0.0.0/24", ""), false},
		{"invalid-ipv4", 2, makeSubnetSet("a.b.c.d/16", ""), true},
		{"valid-ipv6", 2, makeSubnetSet("", "fd02::/112"), false},
		{"invalid-ipv6", 2, makeSubnetSet("", "q293::/112"), true},
		{"valid-dual", 2, makeSubnetSet("10.0.0.0/24", "fd02::/120"), false},
		{"invalid-dual-1", 2, makeSubnetSet("10.0.0.0/24", "q293::/120"), true},
		{"invalid-dual-2", 2, makeSubnetSet("a.b.c.d/16", "fd02::/120"), true},
		{"dual-different-size", 2, makeSubnetSet("10.0.0.0/24", "fd02::/112"), true},
		{"size-bits-0", 0, makeSubnetSet("10.0.0.1/32", ""), false},
		{"too-small-ipv4", 5, makeSubnetSet("10.0.0.0/30", ""), true},
		{"too-small-ipv6", 5, makeSubnetSet("", "fd02::/124"), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.subnetSet.Validate(tc.minSize)
			if tc.expectErr && err == nil {
				t.Error("should be invalid")
			}
			if !tc.expectErr && err != nil {
				t.Error("should be valid:", err)
			}
		})
	}
}

func testSubnetSetIs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		subnetSet   SubnetSet
		isIPv4      bool
		isIPv6      bool
		isDualStack bool
	}{
		{"IPv4", makeSubnetSet("10.0.0.0/24", ""), true, false, false},
		{"IPv6", makeSubnetSet("", "fd02::/120"), false, true, false},
		{"DualStack", makeSubnetSet("10.2.0.0/24", "fd02::/120"), false, false, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.isIPv4 != tc.subnetSet.IsIPv4() {
				t.Error("failed")
			}
			if tc.isIPv6 != tc.subnetSet.IsIPv6() {
				t.Error("failed")
			}
			if tc.isDualStack != tc.subnetSet.IsDualStack() {
				t.Error("failed")
			}
		})
	}

}

func testSubnetSetEqual(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		r1    SubnetSet
		r2    SubnetSet
		equal bool
	}{
		{
			"ipv4-equal",
			makeSubnetSet("10.0.0.0/24", ""),
			makeSubnetSet("10.0.0.0/24", ""),
			true,
		},
		{
			"ipv4-not-equal",
			makeSubnetSet("10.0.0.0/24", ""),
			makeSubnetSet("10.2.0.0/24", ""),
			false,
		},
		{
			"ipv4-extra-ipv6-1",
			makeSubnetSet("10.0.0.0/24", "fd00::/120"),
			makeSubnetSet("10.0.0.0/24", ""),
			false,
		},
		{
			"ipv4-extra-ipv6-2",
			makeSubnetSet("10.0.0.0/24", ""),
			makeSubnetSet("10.0.0.0/24", "fd00::/120"),
			false,
		},
		{
			"ipv6-equal",
			makeSubnetSet("", "fd00::/120"),
			makeSubnetSet("", "fd00::/120"),
			true,
		},
		{
			"ipv6-not-equal",
			makeSubnetSet("", "fd00::/120"),
			makeSubnetSet("", "fd00::/112"),
			false,
		},
		{
			"ipv6-extra-ipv4-1",
			makeSubnetSet("10.0.0.0/24", "fd00::/120"),
			makeSubnetSet("", "fd00::/120"),
			false,
		},
		{
			"ipv6-extra-ipv4-2",
			makeSubnetSet("", "fd00::/120"),
			makeSubnetSet("10.0.0.0/24", "fd00::/120"),
			false,
		},
		{
			"dualstack-equal",
			makeSubnetSet("10.0.0.0/24", "fd02::/120"),
			makeSubnetSet("10.0.0.0/24", "fd02::/120"),
			true,
		},
		{
			"dualstack-not-equal-ipv4",
			makeSubnetSet("10.0.0.0/24", "fd02::/120"),
			makeSubnetSet("10.2.0.0/24", "fd02::/120"),
			false,
		},
		{
			"dualstack-not-equal-ipv6",
			makeSubnetSet("10.0.0.0/24", "fd02::/120"),
			makeSubnetSet("10.0.0.0/24", "fd02::/122"),
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.equal != tc.r1.Equal(tc.r2) {
				t.Error("wrong")
			}
		})
	}
}

func makeSubnet(s string) *net.IPNet {
	_, n, _ := net.ParseCIDR(s)
	return n
}

func testSubnetSetGetBlock(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		r    SubnetSet
		n    uint
		bits int
		ipv4 *net.IPNet
		ipv6 *net.IPNet
	}{
		{
			"ipv4-n0-bits0",
			makeSubnetSet("10.2.0.0/24", ""),
			0, 0,
			makeSubnet("10.2.0.0/32"), nil,
		},
		{
			"ipv4-n3-bits0",
			makeSubnetSet("10.2.0.0/24", ""),
			3, 0,
			makeSubnet("10.2.0.3/32"), nil,
		},
		{
			"ipv4-n3-bits2",
			makeSubnetSet("10.2.0.0/24", ""),
			3, 2,
			makeSubnet("10.2.0.12/30"), nil,
		},
		{
			"ipv6-n0-bits0",
			makeSubnetSet("", "fd02::0900:0000/116"),
			0, 0,
			nil, makeSubnet("fd02::0900:0000/128"),
		},
		{
			"ipv6-n257-bits0",
			makeSubnetSet("", "fd02::0900:0000/116"),
			257, 0,
			nil, makeSubnet("fd02::0900:0101/128"),
		},
		{
			"ipv6-n10-bits5",
			makeSubnetSet("", "fd02::0900:0000/116"),
			10, 5,
			nil, makeSubnet("fd02::0900:0140/123"),
		},
		{
			"dual-n10-bits5",
			makeSubnetSet("10.2.0.0/24", "fd02::0900:0000/120"),
			10, 5,
			makeSubnet("10.2.1.64/27"), makeSubnet("fd02::0900:0140/123"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ipv4, ipv6 := tc.r.GetBlock(tc.n, tc.bits)
			if tc.ipv4 == nil && ipv4 != nil {
				t.Error("unexpected ipv4:", ipv4)
			}
			if tc.ipv4 != nil {
				if ipv4 == nil {
					t.Error("ipv4 must be returned")
				} else {
					if tc.ipv4.String() != ipv4.String() {
						t.Errorf("ipv4 mismatch: expected=%s, actual=%s", tc.ipv4.String(), ipv4.String())
					}
				}
			}
			if tc.ipv6 == nil && ipv6 != nil {
				t.Error("unexpected ipv6:", ipv6)
			}
			if tc.ipv6 != nil {
				if ipv6 == nil {
					t.Error("ipv6 must be returned")
				} else {
					if tc.ipv6.String() != ipv6.String() {
						t.Errorf("ipv4 mismatch: expected=%s, actual=%s", tc.ipv6.String(), ipv6.String())
					}
				}
			}
		})
	}
}
