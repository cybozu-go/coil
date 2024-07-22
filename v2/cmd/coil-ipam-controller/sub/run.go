package sub

import (
	"fmt"
	"net"
	"strconv"
	"time"

	v2 "github.com/cybozu-go/coil/v2"
	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/controllers"
	"github.com/cybozu-go/coil/v2/pkg/indexing"
	"github.com/cybozu-go/coil/v2/pkg/ipam"
	"github.com/cybozu-go/coil/v2/runners"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	gracefulTimeout = 20 * time.Second
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(coilv2.AddToScheme(scheme))

	// +kubebuilder:scaffold:scheme
}

func subMain() error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&config.zapOpts)))

	host, portStr, err := net.SplitHostPort(config.webhookAddr)
	if err != nil {
		return fmt.Errorf("invalid webhook address: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid webhook address: %w", err)
	}

	timeout := gracefulTimeout
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		LeaderElection:          true,
		LeaderElectionID:        "coil-ipam-leader",
		LeaderElectionNamespace: "kube-system", // coil should run in kube-system
		Metrics: metricsserver.Options{
			BindAddress: config.metricsAddr,
		},
		GracefulShutdownTimeout: &timeout,
		HealthProbeBindAddress:  config.healthAddr,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    host,
			Port:    port,
			CertDir: config.certDir,
		}),
	})
	if err != nil {
		return err
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return err
	}

	// register controllers

	pm := ipam.NewPoolManager(mgr.GetClient(), mgr.GetAPIReader(), ctrl.Log.WithName("pool-manager"), scheme)
	apctrl := controllers.AddressPoolReconciler{
		Client:  mgr.GetClient(),
		Scheme:  scheme,
		Manager: pm,
	}
	if err := apctrl.SetupWithManager(mgr); err != nil {
		return err
	}

	ctx := ctrl.SetupSignalHandler()
	if err := indexing.SetupIndexForAddressBlock(ctx, mgr); err != nil {
		return err
	}

	brctrl := controllers.BlockRequestReconciler{
		Client:  mgr.GetClient(),
		Scheme:  scheme,
		Manager: pm,
	}
	if err := brctrl.SetupWithManager(mgr); err != nil {
		return err
	}

	// register webhooks

	if err := (&coilv2.AddressPool{}).SetupWebhookWithManager(mgr); err != nil {
		return err
	}

	// other runners

	gc := runners.NewGarbageCollector(mgr, ctrl.Log.WithName("gc"), config.gcInterval)
	if err := mgr.Add(gc); err != nil {
		return err
	}

	setupLog.Info(fmt.Sprintf("starting manager (version: %s)", v2.Version()))
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		return err
	}

	return nil
}
