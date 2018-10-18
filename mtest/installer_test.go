package mtest

import (
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
	BeforeEach(initializeCoilData)
	AfterEach(cleanCoilData)

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
})
