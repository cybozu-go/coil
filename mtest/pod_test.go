package mtest

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("pod deployment", func() {
	It("should create address pool", func() {
		By("creating address pool")
		_, _, err := coilctl("pool create default 10.0.1.0/24 2")
		Expect(err).NotTo(HaveOccurred())

		_, _, err = coilctl("pool show --json default 10.0.1.0/24")
		Expect(err).NotTo(HaveOccurred())

		By("deployment Pods")

		By("checking ip link")

		By("checking ip route")

		By("checking ip addr")

		By("checking address blocks by node")

	})
})
