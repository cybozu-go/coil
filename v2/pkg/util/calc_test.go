package util

import (
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestIPAdd(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		ip     net.IP
		val    int64
		expect net.IP
	}{
		{
			"small-v4-positive",
			net.ParseIP("0.1.2.3"),
			256,
			net.ParseIP("0.1.3.3").To4(),
		},
		{
			"small-v4-negative",
			net.ParseIP("0.1.2.3"),
			-255,
			net.ParseIP("0.1.1.4").To4(),
		},
		{
			"large-v4-positive",
			net.ParseIP("253.1.2.3"),
			10,
			net.ParseIP("253.1.2.13").To4(),
		},
		{
			"large-v4-negative",
			net.ParseIP("253.1.2.3"),
			-255,
			net.ParseIP("253.1.1.4").To4(),
		},
		{
			"ipv4-invalid-negative",
			net.ParseIP("0.0.0.3"),
			-10,
			nil,
		},
		{
			"ipv4-invalid-positive",
			net.ParseIP("255.255.254.10"),
			1000,
			nil,
		},
		{
			"small-v6-positive",
			net.ParseIP("::1234"),
			256,
			net.ParseIP("::1334"),
		},
		{
			"small-v6-negative",
			net.ParseIP("::1234"),
			-255,
			net.ParseIP("::1135"),
		},
		{
			"large-v6-positive",
			net.ParseIP("fd02::0001"),
			10,
			net.ParseIP("fd02::000b"),
		},
		{
			"large-v6-negative",
			net.ParseIP("fd02::0123"),
			-255,
			net.ParseIP("fd02::0024"),
		},
		{
			"ipv6-invalid-negative",
			net.ParseIP("::0003"),
			-10,
			nil,
		},
		{
			"ipv6-invalid-positive",
			net.ParseIP("ffff:ffff:ffff:ffff:ffff:ffff:ffff:fe00"),
			1000,
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := IPAdd(tc.ip, tc.val)
			if !cmp.Equal(actual, tc.expect) {
				t.Error("fail", cmp.Diff(actual, tc.expect))
			}
		})
	}
}

func TestIPDiff(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		ip1    net.IP
		ip2    net.IP
		expect int64
	}{
		{
			"ipv4-positive",
			net.ParseIP("10.2.0.0"),
			net.ParseIP("10.2.1.3"),
			259,
		},
		{
			"ipv4-negative",
			net.ParseIP("10.2.1.3"),
			net.ParseIP("10.2.0.0"),
			-259,
		},
		{
			"ipv4-zero",
			net.ParseIP("10.2.1.3"),
			net.ParseIP("10.2.1.3"),
			0,
		},
		{
			"ipv6-positive",
			net.ParseIP("fd02::"),
			net.ParseIP("fd02::0102"),
			258,
		},
		{
			"ipv6-negative",
			net.ParseIP("fd02::0102"),
			net.ParseIP("fd02::"),
			-258,
		},
		{
			"ipv6-zero",
			net.ParseIP("fd02::0102"),
			net.ParseIP("fd02::0102"),
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := IPDiff(tc.ip1, tc.ip2)
			if actual != tc.expect {
				t.Error("fail", cmp.Diff(actual, tc.expect))
			}
		})
	}
}
