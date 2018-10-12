package mtest

import (
	"os"
)

var (
	bridgeAddress  = os.Getenv("BRIDGE_ADDRESS")
	host1          = os.Getenv("HOST1")
	node1          = os.Getenv("NODE1")
	node2          = os.Getenv("NODE2")
	sshKeyFile     = os.Getenv("SSH_PRIVKEY")
	ckecliPath     = os.Getenv("CKECLI")
	kubectlPath    = os.Getenv("KUBECTL")
	ckeClusterPath = os.Getenv("CKECLUSTER")
	ckeConfigPath  = os.Getenv("CKECONFIG")
	debug          = os.Getenv("DEBUG") == "1"
)
