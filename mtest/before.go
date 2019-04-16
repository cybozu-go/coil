package mtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cybozu-go/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

const (
	dummyCNIConf = "/etc/cni/net.d/00-dummy.conf"
)

// RunBeforeSuite is for Ginkgo BeforeSuite
func RunBeforeSuite() {
	fmt.Println("Preparing...")

	SetDefaultEventuallyPollingInterval(3 * time.Second)
	SetDefaultEventuallyTimeout(10 * time.Minute)

	log.DefaultLogger().SetThreshold(log.LvError)

	err := prepareSSHClients(host1, node1, node2)
	Expect(err).NotTo(HaveOccurred())

	// sync VM root filesystem to store newly generated SSH host keys.
	for h := range sshClients {
		execSafeAt(h, "sync")
	}

	By("setup Kubernetes with CKE")
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
		return nil
	}).Should(Succeed())

	By("copying test files")
	for _, testFile := range []string{coilctlPath} {
		f, err := os.Open(testFile)
		Expect(err).NotTo(HaveOccurred())
		defer f.Close()
		remoteFilename := filepath.Join("/tmp", filepath.Base(testFile))
		for _, host := range []string{host1} {
			_, err := f.Seek(0, os.SEEK_SET)
			Expect(err).NotTo(HaveOccurred())
			stdout, stderr, err := execAtWithStream(host, f, "dd", "of="+remoteFilename)
			Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
			stdout, stderr, err = execAt(host, "sudo", "mv", remoteFilename, filepath.Join("/opt/bin", filepath.Base(testFile)))
			Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
			stdout, stderr, err = execAt(host, "sudo", "chmod", "755", filepath.Join("/opt/bin", filepath.Base(testFile)))
			Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		}
	}

	By("loading coil image")
	f, err := os.Open(coilImagePath)
	Expect(err).NotTo(HaveOccurred())
	defer f.Close()
	remoteFilename := filepath.Join("/tmp", filepath.Base(coilImagePath))
	for _, host := range []string{node1, node2} {
		_, err := f.Seek(0, os.SEEK_SET)
		Expect(err).NotTo(HaveOccurred())
		stdout, stderr, err := execAtWithStream(host, f, "dd", "of="+remoteFilename)
		Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = execAt(host, "docker", "load", "-i", remoteFilename)
		Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = execAt(host, "sudo", "/opt/bin/ctr", "--address=/var/run/k8s-containerd.sock", "--namespace=k8s.io", "images", "import", remoteFilename)
		Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	}

	By("setup coil-node daemonsets")
	_, stderr, err = execAt(host1, "/data/setup-coil.sh")
	if err != nil {
		fmt.Println("err!!!", string(stderr))
		panic(err)
	}

	fmt.Println("Begin tests...")
}
