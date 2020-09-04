package runners

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var scheme = runtime.NewScheme()

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Runner Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.StacktraceLevel(zapcore.DPanicLevel)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
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

	// prepare resources
	ctx := context.Background()
	node1 := &corev1.Node{}
	node1.Name = "node1"
	err = k8sClient.Create(ctx, node1)
	Expect(err).ToNot(HaveOccurred())
	node1.Status.Addresses = []corev1.NodeAddress{
		{Type: corev1.NodeInternalIP, Address: "10.20.30.41"},
		{Type: corev1.NodeInternalIP, Address: "fd10::41"},
	}
	err = k8sClient.Status().Update(ctx, node1)
	Expect(err).ToNot(HaveOccurred())

	node4 := &corev1.Node{}
	node4.Name = "node4"
	err = k8sClient.Create(ctx, node4)
	Expect(err).ToNot(HaveOccurred())
	node4.Status.Addresses = []corev1.NodeAddress{
		{Type: corev1.NodeInternalIP, Address: "fd10::44"},
	}
	err = k8sClient.Status().Update(ctx, node4)
	Expect(err).ToNot(HaveOccurred())

	node5 := &corev1.Node{}
	node5.Name = "node5"
	err = k8sClient.Create(ctx, node5)
	Expect(err).ToNot(HaveOccurred())
	node5.Status.Addresses = []corev1.NodeAddress{
		{Type: corev1.NodeInternalIP, Address: "10.20.30.45"},
	}
	err = k8sClient.Status().Update(ctx, node5)
	Expect(err).ToNot(HaveOccurred())

	node6 := &corev1.Node{}
	node6.Name = "node6"
	err = k8sClient.Create(ctx, node6)
	Expect(err).ToNot(HaveOccurred())
	node6.Status.Addresses = []corev1.NodeAddress{
		{Type: corev1.NodeInternalIP, Address: "10.20.30.46"},
		{Type: corev1.NodeInternalIP, Address: "fd10::46"},
	}
	err = k8sClient.Status().Update(ctx, node6)
	Expect(err).ToNot(HaveOccurred())

	ns1 := &corev1.Namespace{}
	ns1.Name = "ns1"
	err = k8sClient.Create(ctx, ns1)
	Expect(err).ToNot(HaveOccurred())

	ns2 := &corev1.Namespace{}
	ns2.Name = "ns2"
	ns2.Annotations = map[string]string{
		"coil.cybozu.com/pool": "global",
	}
	err = k8sClient.Create(ctx, ns2)
	Expect(err).ToNot(HaveOccurred())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func deleteAllAddressBlocks() {
	ctx := context.Background()
	blocks := &coilv2.AddressBlockList{}
	err := k8sClient.List(ctx, blocks)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, b := range blocks.Items {
		b.Finalizers = nil
		err := k8sClient.Update(ctx, &b)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	err = k8sClient.DeleteAllOf(ctx, &coilv2.AddressBlock{})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func findMetric(mf *dto.MetricFamily, labels map[string]string) *dto.Metric {
OUTER:
	for _, m := range mf.Metric {
		having := make(map[string]string)
		for _, p := range m.Label {
			having[*p.Name] = *p.Value
		}
		for k, v := range labels {
			if having[k] != v {
				continue OUTER
			}
		}
		return m
	}
	return nil
}
