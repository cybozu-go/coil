package coil

import (
	"encoding/json"
	"net"
	"reflect"
	"testing"
)

func testBlockAssignmentMarshalJSON(t *testing.T) {
	t.Parallel()

	_, ipNet1, err := net.ParseCIDR("10.11.0.0/27")
	if err != nil {
		t.Fatal(err)
	}
	_, ipNet2, err := net.ParseCIDR("10.11.0.32/27")
	if err != nil {
		t.Fatal(err)
	}
	_, ipNet3, err := net.ParseCIDR("10.11.0.64/27")
	if err != nil {
		t.Fatal(err)
	}
	freeList := []*net.IPNet{ipNet1, ipNet2}
	nodes := make(map[string][]*net.IPNet)
	nodes["node1"] = []*net.IPNet{ipNet3}
	pool := BlockAssignment{
		FreeList: freeList,
		Nodes:    nodes,
	}

	data, err := json.Marshal(pool)
	if err != nil {
		t.Fatal(err)
	}

	res := new(BlockAssignment)
	err = json.Unmarshal(data, res)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res.FreeList, freeList) {
		t.Errorf("res.FreeList != freeList: %v != %v", res.FreeList, freeList)
	}
	if !reflect.DeepEqual(res.Nodes, nodes) {
		t.Errorf("res.Nodes != nodes: %v != %v", res.Nodes, nodes)
	}
}

func testBlockAssignmentReleaseBlock(t *testing.T) {
	_, ipNet1, err := net.ParseCIDR("10.11.0.0/27")
	if err != nil {
		t.Fatal(err)
	}
	_, ipNet2, err := net.ParseCIDR("10.11.0.32/27")
	if err != nil {
		t.Fatal(err)
	}
	_, ipNet3, err := net.ParseCIDR("10.11.0.64/27")
	if err != nil {
		t.Fatal(err)
	}
	freeList := []*net.IPNet{ipNet1, ipNet2}
	nodes := make(map[string][]*net.IPNet)
	nodes["node1"] = []*net.IPNet{ipNet3}
	ba := BlockAssignment{
		FreeList: freeList,
		Nodes:    nodes,
	}
	err = ba.ReleaseBlock("node1", ipNet3)
	if err != nil {
		t.Fatal(err)
	}

	expected := BlockAssignment{[]*net.IPNet{ipNet1, ipNet2, ipNet3}, make(map[string][]*net.IPNet)}

	if !reflect.DeepEqual(expected, ba) {
		t.Errorf("expected: %v,\n actual: %v", expected, ba)
	}

	err = ba.ReleaseBlock("node1", ipNet3)
	if err != ErrBlockNotFound {
		t.Fatal("should return ErrBlockNotFound")
	}

}

func TestBlock(t *testing.T) {
	t.Run("Marshaler", testBlockAssignmentMarshalJSON)
	t.Run("Release", testBlockAssignmentReleaseBlock)
}
