package cni

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"
)

// Del free the IP address allocated for a container.
func Del(args *skel.CmdArgs) error {
	// TODO: implement
	return fmt.Errorf("not implemented")
}
