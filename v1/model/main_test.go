package model

import (
	"os"
	"testing"

	"github.com/cybozu-go/coil/v1/test"
)

const (
	clientPort = "22379"
	peerPort   = "22380"
)

func TestMain(m *testing.M) {
	os.Exit(test.RunEtcd(m, clientPort, peerPort))
}
