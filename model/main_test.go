package model

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(TestEtcdRun(m))
}
