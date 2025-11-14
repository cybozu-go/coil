package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestE2e(t *testing.T) {
	if kubectlCmd == "" {
		t.Skip("Use make to run e2e tests")
	}
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(5 * time.Minute)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	RunSpecs(t, "E2e Suite")
}

var kubectlCmd = os.Getenv("KUBECTL")

func kubectl(input []byte, args ...string) (stdout []byte, err error) {
	if kubectlCmd == "" {
		panic("KUBECTL environment variable must be set")
	}

	cmd := exec.Command(kubectlCmd, args...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}

	return cmd.Output()
}

func kubectlSafe(input []byte, args ...string) []byte {
	stdout, err := kubectl(input, args...)
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
	return stdout
}

// ns, name, label are optional.  If name is empty, obj must be a list type.
func getResource(ns, resource, name, label string, obj interface{}) error {
	var args []string
	if ns != "" {
		args = append(args, "-n", ns)
	}
	args = append(args, "get", resource)
	if name != "" {
		args = append(args, name)
	}
	if label != "" {
		args = append(args, "-l", label)
	}
	args = append(args, "-o", "json")
	data, err := kubectl(nil, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, obj)
}

// ns, name, label are optional.  If name is empty, obj must be a list type.
func getResourceSafe(ns, resource, name, label string, obj interface{}) {
	err := getResource(ns, resource, name, label, obj)
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
}

func runOnNode(node, cmd string, args ...string) (stdout []byte, err error) {
	dockerArgs := append([]string{"exec", node, cmd}, args...)
	return exec.Command("docker", dockerArgs...).Output()
}

func runOnPod(namespace, name string, args ...string) []byte {
	command := []string{"exec", "-i", "-n", namespace, name, "--"}
	command = append(command, args...)
	return kubectlSafe(nil, command...)
}
