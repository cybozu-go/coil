package main

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/cybozu-go/coil/v2/pkg/constants"
)

// PluginEnvArgs represents CNI_ARG
type PluginEnvArgs struct {
	types.CommonArgs
	K8S_POD_NAMESPACE          types.UnmarshallableString
	K8S_POD_NAME               types.UnmarshallableString
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}

// Map returns a map[string]string
func (e PluginEnvArgs) Map() map[string]string {
	return map[string]string{
		constants.PodNamespaceKey: string(e.K8S_POD_NAMESPACE),
		constants.PodNameKey:      string(e.K8S_POD_NAME),
		constants.PodContainerKey: string(e.K8S_POD_INFRA_CONTAINER_ID),
	}
}

// PluginConf represents JSON netconf for Coil.
type PluginConf struct {
	types.NetConf

	// Coil specific flags
	Socket string `json:"socket"`
}

func parseConfig(stdin []byte) (*PluginConf, error) {
	conf := &PluginConf{
		Socket: constants.DefaultSocketPath,
	}

	if err := json.Unmarshal(stdin, conf); err != nil {
		return nil, fmt.Errorf("failed to parse network configuration: %w", err)
	}

	if err := version.ParsePrevResult(&conf.NetConf); err != nil {
		return nil, fmt.Errorf("failed to parse prev result: %w", err)
	}

	return conf, nil
}
