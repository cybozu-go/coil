//go:build privileged

package test

import (
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTest(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test Suite")
}

// runCmd executes a command and fails the test if it returns an error.
// This helper is useful for setup/teardown operations in integration tests.
func runCmd(name string, args ...string) {
	out, err := exec.Command(name, args...).CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "command failed: %s %v: %s", name, args, string(out))
}
