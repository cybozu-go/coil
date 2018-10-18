package mtest

import (
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	setSysctlParams = map[string]string{
		"net.ipv4.ip_forward":          "1",
		"net.ipv6.conf.all.forwarding": "1",
	}
)

var _ = Describe("coil-installer", func() {
	BeforeEach(initializeCoil)
	AfterEach(cleanCoil)

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
