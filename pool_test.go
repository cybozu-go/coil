package coil

import (
	"encoding/json"
	"net"
	"reflect"
	"testing"
)

func testAddressPoolMarshalJSON(t *testing.T) {
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
	t.Run("Marsheler", testAddressPoolMarshalJSON)
}
