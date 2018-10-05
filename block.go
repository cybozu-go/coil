package coil

// BlockAssignment holds address block assignment information for a subnet
type BlockAssignment struct {
	FreeList []string            `json:"free"`
	Nodes    map[string][]string `json:"nodes"`
}
