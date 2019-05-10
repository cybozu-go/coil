package coild

import (
	"os"
	"testing"

	"github.com/cybozu-go/coil/model"
)

func TestMain(m *testing.M) {
	os.Exit(model.TestEtcdRun(m))
}
