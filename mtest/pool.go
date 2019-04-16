package mtest

import (
	. "github.com/onsi/ginkgo"
)

// TestPool tests address pool management
func TestPool() {
	BeforeEach(initializeCoil)
	AfterEach(cleanCoil)

	It("should create address pool", func() {
		By("creating address pool")
		coilctlSafe("pool", "create", "test1", "10.0.3.0/24", "2")
		coilctlSafe("pool", "show", "--json", "test1", "10.0.3.0/24")

		By("checking add-subnet to existing pool")
		coilctlSafe("pool", "add-subnet", "test1", "10.0.4.0/24")
		coilctlSafe("pool", "show", "--json", "test1", "10.0.4.0/24")
	})
}
