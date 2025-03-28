package cert

import (
	"context"
	"fmt"

	"github.com/open-policy-agent/cert-controller/pkg/rotator"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;update

func SetupRotator(mgr ctrl.Manager, objectType string, enableRestartOnCertRefresh bool, setupFinished chan struct{}) (chan struct{}, error) {
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
		CertDir:                "/certs",
		CAName:                 fmt.Sprintf("coil-%s-ca", objectType),
		CAOrganization:         "coil",
		DNSName:                fmt.Sprintf("%s.%s.svc", serviceName, podNamespace),
		ExtraDNSNames:          []string{fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, podNamespace)},
		IsReady:                setupFinished,
		RequireLeaderElection:  true,
		Webhooks:               webhooks,
		RestartOnSecretRefresh: enableRestartOnCertRefresh,
	}); err != nil {
		return nil, fmt.Errorf("unable to set up cert rotation: %w", err)
	}

	return setupFinished, nil
}

func WaitForExit(setupErr, mgrErr chan error, cancel context.CancelFunc) error {
	logger := ctrl.Log.WithName("coil")
	for {
		select {
		case err := <-setupErr:
			if err != nil {
				logger.Error(err, "unable to setup reconcilers")
				cancel() // if error occurred during setup cancel manager's context
			}
		case err := <-mgrErr:
			if err != nil {
				return fmt.Errorf("manager error: %w", err)
			}
			return nil
		}
	}
}
