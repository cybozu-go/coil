package controllers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	// +kubebuilder:scaffold:imports
)

func strPtr(s string) *string {
	return &s
}

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var scheme = runtime.NewScheme()

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(20 * time.Second)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	err = coilv2.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	// prepare resources
	ctx := context.Background()
	ap := &coilv2.AddressPool{}
	ap.Name = "default"
	ap.Spec.BlockSizeBits = 1
	ap.Spec.Subnets = []coilv2.SubnetSet{
		{IPv4: strPtr("10.2.0.0/29"), IPv6: strPtr("fd02::0200/125")},
		{IPv4: strPtr("10.3.0.0/30"), IPv6: strPtr("fd02::0300/126")},
	}
	err = k8sClient.Create(ctx, ap)
	Expect(err).ToNot(HaveOccurred())

	ap = &coilv2.AddressPool{}
	ap.Name = "v4"
	ap.Spec.BlockSizeBits = 2
	ap.Spec.Subnets = []coilv2.SubnetSet{
		{IPv4: strPtr("10.4.0.0/29")},
	}
	err = k8sClient.Create(ctx, ap)
	Expect(err).ToNot(HaveOccurred())

	node1 := &corev1.Node{}
	node1.Name = "node1"
	err = k8sClient.Create(ctx, node1)
	Expect(err).ToNot(HaveOccurred())

	cr := &rbacv1.ClusterRole{}
	cr.Name = "coil-egress"
	err = k8sClient.Create(ctx, cr)
	Expect(err).ToNot(HaveOccurred())

	crb := &rbacv1.ClusterRoleBinding{}
	crb.Name = "coil-egress"
	crb.RoleRef.APIGroup = rbacv1.SchemeGroupVersion.Group
	crb.RoleRef.Kind = "ClusterRole"
	crb.RoleRef.Name = "coil-egress"
	err = k8sClient.Create(ctx, crb)
	Expect(err).ToNot(HaveOccurred())

	ns := &corev1.Namespace{}
	ns.Name = "egtest"
	err = k8sClient.Create(ctx, ns)
	Expect(err).ToNot(HaveOccurred())

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
