package sub

import (
	"errors"
	"os"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/controllers"
	"github.com/cybozu-go/coil/v2/pkg/ipam"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	gracefulTimeout = 20 * time.Second
	nodeEnvName     = "COIL_NODE_NAME"
)

func subMain() error {
	log := ctrl.Log.WithName("main")
	nodeName := os.Getenv(nodeEnvName)
	if nodeName == "" {
		return errors.New(nodeEnvName + " environment variable should be set")
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return err
	}
	if err := coilv2.AddToScheme(scheme); err != nil {
		return err
	}

	ctrl.SetLogger(zap.New(zap.StacktraceLevel(zapcore.DPanicLevel)))

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

	exporter := nodenet.NewRouteExporter(config.tableId, config.protocolId, ctrl.Log.WithName("route-exporter"))
	nodeIPAM := ipam.NewNodeIPAM(nodeName, ctrl.Log.WithName("node-ipam"), mgr, exporter)
	watcher := &controllers.BlockRequestWatcher{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("blockrequest-watcher"),
		NodeIPAM: nodeIPAM,
		Scheme:   mgr.GetScheme(),
	}
	err = watcher.SetupWithManager(mgr)
	if err != nil {
		return err
	}

	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "problem running manager")
		return err
	}

	return nil
}
