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

	nodes = make(map[string][]*net.IPNet)
	nodes["node1"] = []*net.IPNet{ipNet1, ipNet2, ipNet3}
	ba = BlockAssignment{
		Nodes: nodes,
	}

	err = ba.ReleaseBlock("node1", ipNet2)
	if err != nil {
		t.Fatal(err)
	}

	expected = BlockAssignment{[]*net.IPNet{ipNet2}, map[string][]*net.IPNet{
		"node1": {ipNet1, ipNet3},
	}}

	if !reflect.DeepEqual(expected, ba) {
		t.Errorf("expected: %v,\n actual: %v", expected, ba)
	}

	err = ba.ReleaseBlock("node1", ipNet1)
	if err != nil {
		t.Fatal(err)
	}

	expected = BlockAssignment{[]*net.IPNet{ipNet2, ipNet1}, map[string][]*net.IPNet{
		"node1": {ipNet3},
	}}

	if !reflect.DeepEqual(expected, ba) {
		t.Errorf("expected: %v,\n actual: %v", expected, ba)
	}
}

func testEmptyAssignment(t *testing.T) {
	_, subnet, _ := net.ParseCIDR("10.20.0.0/24")
	assignment := EmptyAssignment(subnet, 5)
	free := []string{
		"10.20.0.0/27", "10.20.0.32/27", "10.20.0.64/27", "10.20.0.96/27",
		"10.20.0.128/27", "10.20.0.160/27", "10.20.0.192/27", "10.20.0.224/27",
	}

	if len(assignment.FreeList) != len(free) {
		t.Fatalf("len(assignment.FreeList) != len(free), %d != %d", len(assignment.FreeList), len(free))
	}

	t.Log(assignment.FreeList)
	for i, ipnet := range assignment.FreeList {
		if ipnet.String() != free[i] {
			t.Errorf("ipnet.String() != free[i]: %s != %s", ipnet.String(), free[i])
		}

	}
}

func TestBlock(t *testing.T) {
	t.Run("Marshaler", testBlockAssignmentMarshalJSON)
	t.Run("Release", testBlockAssignmentReleaseBlock)
	t.Run("EmptyAssignment", testEmptyAssignment)
}
