package coil

import (
	"encoding/json"
	"net"
	"reflect"
	"testing"
)

func testAddressPoolValidate(t *testing.T) {
	t.Parallel()

	_, n1, err := net.ParseCIDR("10.11.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	_, n2, err := net.ParseCIDR("10.12.0.0/24")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name  string
		pool  AddressPool
		valid bool
	}{
		{
			name:  "no subnets",
			pool:  AddressPool{Subnets: nil},
			valid: false,
		},
		{
			name:  "too small block size",
			pool:  AddressPool{Subnets: []*net.IPNet{n1}},
			valid: false,
		},
		{
			name:  "too large block size",
			pool:  AddressPool{Subnets: []*net.IPNet{n2}, BlockSize: 10},
			valid: false,
		},
		{
			name:  "exact size",
			pool:  AddressPool{Subnets: []*net.IPNet{n2}, BlockSize: 8},
			valid: true,
		},
		{
			name:  "all ok",
			pool:  AddressPool{Subnets: []*net.IPNet{n1, n2}, BlockSize: 5},
			valid: true,
		},
	}

	for _, c := range testCases {
		err := c.pool.Validate()
		if err != nil && c.valid {
			t.Errorf("%s: expected valid, but: %v", c.name, err)
			continue
		}
		if err == nil && !c.valid {
			t.Errorf("%s: expected invalid, but valid", c.name)
		}
	}
}

func testAddressPoolMarshalJSON(t *testing.T) {
	t.Parallel()

	_, ipNet1, err := net.ParseCIDR("10.11.0.0/27")
	if err != nil {
		t.Fatal(err)
	}
	_, ipNet2, err := net.ParseCIDR("10.11.0.32/27")
	if err != nil {
		t.Fatal(err)
	}
	subnets := []*net.IPNet{ipNet1, ipNet2}
	pool := AddressPool{
		Subnets:   subnets,
		BlockSize: 5,
	}

	data, err := json.Marshal(pool)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(string(data))

	res := new(AddressPool)
	err = json.Unmarshal(data, res)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res.Subnets, subnets) {
		t.Errorf("res.Subnets != subnets: %v != %v", res.Subnets, subnets)
	}
}

func TestPool(t *testing.T) {
	t.Run("Validate", testAddressPoolValidate)
	t.Run("Marsheler", testAddressPoolMarshalJSON)
}
