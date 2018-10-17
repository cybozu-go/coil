package cni

import (
	"encoding/json"
	"net/url"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
)

// Del free the IP address allocated for a container.
func Del(args *skel.CmdArgs) error {
	conf := new(PluginConf)
	err := json.Unmarshal(args.StdinData, conf)
	if err != nil {
		return err
	}
	coildURL, err := url.Parse(conf.CoildURL)
	if err != nil {
		return err
	}

	kv := parseArgs(args.Args)
	podNS, podName, err := getPodInfo(kv)
	if err != nil {
		return err
	}

	err = returnIPToCoild(coildURL, podNS, podName)
	if err != nil {
		return err
	}

	// Kubernetes sends multiple DEL for dead containers.
	// See: https://github.com/kubernetes/kubernetes/issues/44100
	if args.Netns == "" {
		return nil
	}

	return ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		err := ip.DelLinkByName(args.IfName)
		if err == nil || err == ip.ErrLinkNotFound {
			return nil
		}
		return err
	})
}
