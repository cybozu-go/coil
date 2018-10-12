package mtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func TestMtest(t *testing.T) {
	if len(sshKeyFile) == 0 {
		t.Skip("no SSH_PRIVKEY envvar")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Multi-host test for cke")
}

var _ = BeforeSuite(func() {
	fmt.Println("Preparing...")

	SetDefaultEventuallyPollingInterval(3 * time.Second)
	SetDefaultEventuallyTimeout(6 * time.Minute)

	err := prepareSSHClients(host1, node1, node2)
	Expect(err).NotTo(HaveOccurred())

	// sync VM root filesystem to store newly generated SSH host keys.
	for h := range sshClients {
		execSafeAt(h, "sync")
	}

	// load coil container images
	data, err := ioutil.ReadFile(coilImagePath)
	if err != nil {
		panic(err)
	}
	err = execAtWithInput(node1, data, "docker", "load")
	if err != nil {
		panic(err)
	}
	err = execAtWithInput(node2, data, "docker", "load")
	if err != nil {
		panic(err)
	}

	// wait cke
	_, stderr, err := execAt(host1, "/data/setup-cke.sh")
	if err != nil {
		fmt.Println("err!!!", string(stderr))
		panic(err)
	}

	Eventually(func() error {
		stdout, _, err := kubectl("get", "nodes", "-o=json")
		if err != nil {
			return err
		}
		var nl corev1.NodeList
		err = json.Unmarshal(stdout, &nl)
		if err != nil {
			return err
		}
		if len(nl.Items) != 2 {
			return errors.New("Node is not 2")
		}
		for _, node := range nl.Items {
			if !isNodeReady(node) {
				return errors.New("node is not ready")
			}
		}
		return nil
	}).Should(Succeed())

	fmt.Println("Begin tests...")
})
