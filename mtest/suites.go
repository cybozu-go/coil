package mtest

import . "github.com/onsi/ginkgo"

// FunctionsSuite is a test suite that tests small test cases
var FunctionsSuite = func() {
	Context("coil-controller", TestCoilController)
	Context("coil-installer", TestCoilInstaller)
	Context("pod", TestPod)
	Context("pool", TestPool)
}
