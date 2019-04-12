package mtest

import (
	"os"
)

var (
	bridgeAddress = os.Getenv("BRIDGE_ADDRESS")
	host1         = os.Getenv("HOST1")
	node1         = os.Getenv("NODE1")
	node2         = os.Getenv("NODE2")

	ckeConfigPath = os.Getenv("CKECONFIG")
	coilImagePath = os.Getenv("COILIMAGE")
	coilctlPath   = os.Getenv("COILCTL")
	kubectlPath   = os.Getenv("KUBECTL")

	sshKeyFile = os.Getenv("SSH_PRIVKEY")
)
