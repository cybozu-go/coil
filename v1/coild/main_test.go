package coild

import (
	"os"
	"testing"

	"github.com/cybozu-go/coil/v1/test"
)

const (
	clientPort = "12379"
	peerPort   = "12380"
)

func TestMain(m *testing.M) {
	os.Exit(test.RunEtcd(m, clientPort, peerPort))
}
