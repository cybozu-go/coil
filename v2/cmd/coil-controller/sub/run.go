package sub

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/controllers"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/ipam"
	"github.com/cybozu-go/coil/v2/runners"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	gracefulTimeout = 20 * time.Second
)

func subMain() error {
	ctrl.SetLogger(zap.New(zap.StacktraceLevel(zapcore.DPanicLevel)))

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return err
	}
	if err := coilv2.AddToScheme(scheme); err != nil {
		return err
	}

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
		LeaderElectionID:        "coil-leader",
		LeaderElectionNamespace: "kube-system", // coil should run in kube-system
		MetricsBindAddress:      config.metricsAddr,
		GracefulShutdownTimeout: &timeout,
		HealthProbeBindAddress:  config.healthAddr,
		Host:                    host,
		Port:                    port,
		CertDir:                 config.certDir,
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

	pm := ipam.NewPoolManager(mgr.GetClient(), ctrl.Log.WithName("pool-manager"), scheme)
	apctrl := controllers.AddressPoolReconciler{
		Client:  mgr.GetClient(),
		Log:     ctrl.Log.WithName("addresspool-reconciler"),
		Scheme:  scheme,
		Manager: pm,
	}
	if err := apctrl.SetupWithManager(mgr); err != nil {
		return err
	}
	if err := ipam.SetupIndexer(context.Background(), mgr); err != nil {
		return err
	}

	brctrl := controllers.BlockRequestReconciler{
		Client:  mgr.GetClient(),
		Log:     ctrl.Log.WithName("blockrequest-reconciler"),
		Scheme:  scheme,
		Manager: pm,
	}
	if err := brctrl.SetupWithManager(mgr); err != nil {
		return err
	}

	podNS := os.Getenv(constants.EnvPodNamespace)
	podName := os.Getenv(constants.EnvPodName)
	img, err := controllers.GetImage(mgr.GetAPIReader(), client.ObjectKey{Namespace: podNS, Name: podName})
	if err != nil {
		return err
	}
	egressctrl := controllers.EgressReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("egress-reconciler"),
		Scheme: scheme,
		Image:  img,
		Port:   config.egressPort,
	}
	if err := egressctrl.SetupWithManager(mgr); err != nil {
		return err
	}

	// register webhooks

	if err := (&coilv2.AddressPool{}).SetupWebhookWithManager(mgr); err != nil {
		return err
	}
	if err := (&coilv2.Egress{}).SetupWebhookWithManager(mgr); err != nil {
		return err
	}

	// other runners

	gc := runners.NewGarbageCollector(mgr, ctrl.Log.WithName("gc"), config.gcInterval)
	if err := mgr.Add(gc); err != nil {
		return err
	}

	log := ctrl.Log.WithName("main")
	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "problem running manager")
		return err
	}

	return nil
}
