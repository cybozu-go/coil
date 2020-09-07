package sub

import (
	"errors"
	"os"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/controllers"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	"github.com/cybozu-go/coil/v2/runners"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	gracefulTimeout = 5 * time.Second
)

func subMain() error {
	ctrl.SetLogger(zap.New(zap.StacktraceLevel(zapcore.DPanicLevel)))

	nodeName := os.Getenv(constants.EnvNode)
	if nodeName == "" {
		return errors.New(constants.EnvNode + " environment variable must be set")
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return err
	}
	if err := coilv2.AddToScheme(scheme); err != nil {
		return err
	}

	timeout := gracefulTimeout
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		LeaderElection:          false,
		MetricsBindAddress:      config.metricsAddr,
		GracefulShutdownTimeout: &timeout,
		HealthProbeBindAddress:  config.healthAddr,
	})
	if err != nil {
		return err
	}

	notifyCh := make(chan struct{}, 1)
	abr := &controllers.AddressBlockReconciler{Notify: notifyCh}
	if err := abr.SetupWithManager(mgr); err != nil {
		return err
	}

	syncer := nodenet.NewRouteSyncer(config.protocolId, ctrl.Log.WithName("route-syncer"))
	router := runners.NewRouter(mgr, ctrl.Log.WithName("router"), nodeName, notifyCh, syncer, config.updateInterval)
	if err := mgr.Add(router); err != nil {
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
