package mtest

import (
	"os"
)

var (
	bridgeAddress = os.Getenv("BRIDGE_ADDRESS")
	host1         = os.Getenv("HOST1")
	node1         = os.Getenv("NODE1")
	node2         = os.Getenv("NODE2")
	sshKeyFile    = os.Getenv("SSH_PRIVKEY")
	ckecliPath    = os.Getenv("CKECLI")
	ckeConfigPath = os.Getenv("CKECONFIG")
	kubectlPath   = os.Getenv("KUBECTL")
	coilImagePath = os.Getenv("COILIMAGE")
	debug         = os.Getenv("DEBUG") == "1"
)
