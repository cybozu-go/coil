package ipam

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var mgr manager.Manager
var ctx = context.Background()
var cancel context.CancelFunc
var scheme = runtime.NewScheme()

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "IPAM Suite")
}

func strPtr(s string) *string {
	return &s
}

func cleanBlocks() {
	blocks := &coilv2.AddressBlockList{}
	err := k8sClient.List(ctx, blocks)
	Expect(err).ToNot(HaveOccurred())
	for _, block := range blocks.Items {
		block.Finalizers = nil
		err = k8sClient.Update(ctx, &block)
		Expect(err).ToNot(HaveOccurred())
	}
	err = k8sClient.DeleteAllOf(ctx, &coilv2.AddressBlock{})
	Expect(err).ToNot(HaveOccurred())
	time.Sleep(10 * time.Millisecond)
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.StacktraceLevel(zapcore.DPanicLevel)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	err = coilv2.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	// prepare manager
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).ToNot(HaveOccurred())

	ctx := context.Background()
	err = SetupIndexer(ctx, mgr)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err := mgr.Start(ctx)
		if err != nil {
			panic(err)
		}
	}()

	// prepare resources
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

	node2 := &corev1.Node{}
	node2.Name = "node2"
	err = k8sClient.Create(ctx, node2)
	Expect(err).ToNot(HaveOccurred())

})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
