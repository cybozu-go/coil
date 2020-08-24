package main

import (
	"testing"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/cybozu-go/coil/v2/pkg/constants"
)

func TestPluginEnvArgs(t *testing.T) {
	env := &PluginEnvArgs{}

	args := "IgnoreUnknown=1;K8S_POD_NAMESPACE=test;K8S_POD_NAME=testhttpd-host1;K8S_POD_INFRA_CONTAINER_ID=c8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a"
	if err := types.LoadArgs(args, env); err != nil {
		t.Fatal(err)
	}

	if env.K8S_POD_NAMESPACE != "test" {
		t.Error(`env.K8S_POD_NAMESPACE != "test"`, env.K8S_POD_NAMESPACE)
	}

	if env.K8S_POD_NAME != "testhttpd-host1" {
		t.Error(`env.K8S_POD_NAME != "testhttpd-host1"`, env.K8S_POD_NAME)
	}

	if env.K8S_POD_INFRA_CONTAINER_ID != "c8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a" {
		t.Error(`env.K8S_POD_INFRA_CONTAINER_ID != "c8f4a9c50c85b36eff718aab2ac39209e541a4551420488c33d9216cf1795b3a"`, env.K8S_POD_INFRA_CONTAINER_ID)
	}
}

func TestParseConfig(t *testing.T) {
	conf := []byte(`
{
	"cniVersion": "0.4.0",
	"name": "k8s",
	"type": "coil"
}
`)

	pc, err := parseConfig(conf)
	if err != nil {
		t.Fatal(err)
	}
	if pc.CNIVersion != "0.4.0" {
		t.Error(`pc.CNIVersion != "0.4.0"`)
	}
	if pc.Name != "k8s" {
		t.Error(`pc.Name != "k8s"`)
	}
	if pc.Type != "coil" {
		t.Error(`pc.Type != "coil"`)
	}
	if pc.PrevResult != nil {
		t.Error("pc.Result should be nil")
	}
	if pc.Socket != constants.DefaultSocketPath {
		t.Error(`pc.Socket != constants.DefaultSocketPath`)
	}

	conf = []byte(`
{
	"cniVersion": "0.4.0",
	"name": "k8s",
	"type": "coil",
	"socket": "/tmp/coild.sock"
}
`)
	pc, err = parseConfig(conf)
	if err != nil {
		t.Fatal(err)
	}
	if pc.Socket != "/tmp/coild.sock" {
		t.Error(`pc.Socket != "/tmp/coild.sock"`)
	}

	conf = []byte(`
{
  "cniVersion": "0.4.0",
  "name": "k8s",
  "type": "coil",
  "prevResult": {
    "ips": [
        {
          "version": "4",
          "address": "10.0.0.5/32",
          "interface": 2
        }
    ],
    "interfaces": [
        {
            "name": "cni0",
            "mac": "00:11:22:33:44:55"
        },
        {
            "name": "veth3243",
            "mac": "55:44:33:22:11:11"
        },
        {
            "name": "eth0",
            "mac": "99:88:77:66:55:44",
            "sandbox": "/var/run/netns/blue"
        }
    ],
    "dns": {
      "nameservers": [ "10.1.0.1" ]
    }
  }
}`)

	pc, err = parseConfig(conf)
	if err != nil {
		t.Fatal(err)
	}
	if pc.PrevResult == nil {
		t.Error("pc.Result should not be nil")
	}
}
