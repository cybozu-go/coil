package cert

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/cert-controller/pkg/rotator"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;update

func SetupRotator(mgr ctrl.Manager, objectType string, enableRestartOnCertRefresh bool, certDir string) error {
	webhooks := []rotator.WebhookInfo{
		{
			Name: fmt.Sprintf("coilv2-validating-%s-webhook-configuration", objectType),
			Type: rotator.Validating,
		},
		{
			Name: fmt.Sprintf("coilv2-mutating-%s-webhook-configuration", objectType),
			Type: rotator.Mutating,
		},
	}

	podNamespace := "kube-system"
	serviceName := fmt.Sprintf("coilv2-%s-webhook-service", objectType)
	secretName := fmt.Sprintf("coilv2-%s-webhook-server-cert", objectType)

	if err := rotator.AddRotator(mgr, &rotator.CertRotator{
		SecretKey: types.NamespacedName{
			Namespace: podNamespace,
			Name:      secretName,
		},
		CertDir:                certDir,
		CAName:                 fmt.Sprintf("coil-%s-ca", objectType),
		CAOrganization:         "coil",
		DNSName:                fmt.Sprintf("%s.%s.svc", serviceName, podNamespace),
		ExtraDNSNames:          []string{fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, podNamespace)},
		IsReady:                make(chan struct{}),
		RequireLeaderElection:  false,
		Webhooks:               webhooks,
		RestartOnSecretRefresh: enableRestartOnCertRefresh,
	}); err != nil {
		return fmt.Errorf("unable to set up cert rotation: %w", err)
	}

	return nil
}

// Reloader loads the webhook server certificate lazily on TLS handshakes
// and reloads it when the certificate file is updated.  Unlike
// controller-runtime's certwatcher, it tolerates certificate files that do
// not exist yet at server startup; TLS handshakes just fail until the files
// appear (e.g. until kubelet syncs the Secret updated by the cert rotator).
type Reloader struct {
	certPath string
	keyPath  string
	log      logr.Logger

	mu      sync.Mutex
	cert    *tls.Certificate
	modTime time.Time
}

// NewReloader creates a Reloader that loads tls.crt and tls.key from dir.
func NewReloader(dir string, log logr.Logger) *Reloader {
	return &Reloader{
		certPath: filepath.Join(dir, "tls.crt"),
		keyPath:  filepath.Join(dir, "tls.key"),
		log:      log,
	}
}

// GetCertificate is intended for tls.Config.GetCertificate.
// Once a certificate has been loaded, it keeps serving the cached one
// even if the files disappear or become corrupt afterwards.
func (r *Reloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, err := os.Stat(r.certPath)
	if err == nil && (r.cert == nil || !st.ModTime().Equal(r.modTime)) {
		if cert, lerr := tls.LoadX509KeyPair(r.certPath, r.keyPath); lerr == nil {
			r.cert = &cert
			r.modTime = st.ModTime()
		} else {
			err = lerr
		}
	}
	if r.cert == nil {
		return nil, fmt.Errorf("webhook certificate is not ready: %w", err)
	}
	if err != nil {
		r.log.Error(err, "failed to reload the webhook certificate; keep serving the cached one")
	}
	return r.cert, nil
}

// TLSOpts is intended for webhook.Options.TLSOpts.  It makes the webhook
// server obtain its certificate from the Reloader.
func (r *Reloader) TLSOpts() []func(*tls.Config) {
	return []func(*tls.Config){
		func(c *tls.Config) { c.GetCertificate = r.GetCertificate },
	}
}
