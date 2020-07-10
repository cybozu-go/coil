package v2

import (
	"testing"
)

func TestSubnetSet(t *testing.T) {
	t.Run("Validate", testSubnetSetValidate)
	t.Run("Is", testSubnetSetIs)
	t.Run("Equal", testSubnetSetEqual)
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
