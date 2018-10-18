package mtest

import (
	. "github.com/onsi/ginkgo"
)

var _ = Describe("address pool", func() {
	It("should create address pool", func() {
		By("creating address pool")
		coilctl("pool", "create", "test1", "10.0.3.0/24", "2")
		coilctl("pool", "show", "--json", "test1", "10.0.3.0/24")

		By("checking add-subnet to existing pool")
		coilctl("pool", "add-subnet", "test1", "10.0.4.0/24")
		coilctl("pool", "show", "--json", "test1", "10.0.4.0/24")
	})
})
