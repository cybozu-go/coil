package cni

import "github.com/containernetworking/cni/pkg/types"

// PluginConf is configuration for this plugin.
type PluginConf struct {
	types.NetConf
	CoildURL string `json:"coild"`
}
