package v2

import (
	"crypto/tls"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var stopCh = make(chan struct{})

func TestWebhooks(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Coil v2 Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			DirectoryPaths: []string{filepath.Join("..", "..", "config", "webhook")},
		},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	scheme := runtime.NewScheme()
	err = AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	// start webhook server using Manager
	wio := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:         scheme,
		Host:           wio.LocalServingHost,
		Port:           wio.LocalServingPort,
		CertDir:        wio.LocalServingCertDir,
		LeaderElection: false,
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&AddressPool{}).SetupWebhookWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())
	err = (&Egress{}).SetupWebhookWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err := mgr.Start(stopCh)
		if err != nil {
			panic(err)
		}
	}()

	// wait for the webhook server to get ready
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", wio.LocalServingHost, wio.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}).Should(Succeed())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	close(stopCh)
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
