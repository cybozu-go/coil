package mtest

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("address pool", func() {
	It("should create address pool", func() {
		By("creating address pool")
		_, _, err := coilctl("pool create test1 10.0.3.0/24 2")
		Expect(err).NotTo(HaveOccurred())

		_, _, err = coilctl("pool show --json test1 10.0.3.0/24")
		Expect(err).NotTo(HaveOccurred())

		By("checking add-subnet to existing pool")
		_, _, err = coilctl("pool add-subnet test1 10.0.4.0/24")
		Expect(err).NotTo(HaveOccurred())

		_, _, err = coilctl("pool show --json test1 10.0.4.0/24")
		Expect(err).NotTo(HaveOccurred())
	})
})
