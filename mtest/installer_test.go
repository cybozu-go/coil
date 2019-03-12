package mtest

import (
	"encoding/json"
	"errors"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var (
	installedFiles = []string{
		"/etc/cni/net.d/10-coil.conflist",
		"/opt/cni/bin/coil",
	}
	setSysctlParams = map[string]string{
		"net.ipv4.ip_forward":          "1",
		"net.ipv6.conf.all.forwarding": "1",
	}
)

var _ = Describe("coil-installer", func() {
	BeforeEach(initializeCoil)
	AfterEach(cleanCoil)

	It("", func() {
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
	})

	It("should installed files by coil-installer container", func() {
		for _, host := range []string{node1, node2} {
			for _, file := range installedFiles {
				By("checking " + file + " exists at " + host)
				checkFileExists(host, file)
			}
		}
	})

	It("should IP forwarding is enabled by coil-installer container", func() {
		for _, host := range []string{node1, node2} {
			for param, expected := range setSysctlParams {
				By("checking " + param + " sets at " + host)
				result := checkSysctlParam(host, param)
				Expect(strings.TrimSpace(result)).To(Equal(expected))
			}
		}
	})

	It("should remove bootstrap taint from the node where coil is installed", func() {
		stdout, _, err := kubectl("get", "nodes", "-o=json")
		Expect(err).ShouldNot(HaveOccurred())

		nodes := new(corev1.NodeList)
		err = json.Unmarshal(stdout, nodes)
		Expect(err).ShouldNot(HaveOccurred())

		for _, node := range nodes.Items {
			taintKeys := make([]string, len(node.Spec.Taints))
			for _, taint := range node.Spec.Taints {
				taintKeys = append(taintKeys, taint.Key)
			}
			Expect(taintKeys).ShouldNot(ContainElement("coil.cybozu.com/bootstrap"))
		}
	})
})
