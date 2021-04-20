package sub

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/controllers"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/ipam"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	"github.com/cybozu-go/coil/v2/runners"
	"github.com/go-logr/zapr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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
	// coild needs a raw zap logger for grpc_zip.
	zapLogger := zap.NewRaw(zap.UseFlagOptions(&config.zapOpts))
	defer zapLogger.Sync()

	grpcLogger := zapLogger.Named("grpc")
	ctrl.SetLogger(zapr.NewLogger(zapLogger))

	nodeName := os.Getenv(constants.EnvNode)
	if nodeName == "" {
		return errors.New(constants.EnvNode + " environment variable should be set")
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

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return err
	}

	exporter := nodenet.NewRouteExporter(config.exportTableId, config.protocolId, ctrl.Log.WithName("route-exporter"))
	nodeIPAM := ipam.NewNodeIPAM(nodeName, ctrl.Log.WithName("node-ipam"), mgr, exporter)
	watcher := &controllers.BlockRequestWatcher{
		Client:   mgr.GetClient(),
		NodeIPAM: nodeIPAM,
		NodeName: nodeName,
	}
	if err := watcher.SetupWithManager(mgr); err != nil {
		return err
	}

	podNet := nodenet.NewPodNetwork(
		config.podTableId,
		config.podRulePrio,
		config.protocolId,
		config.compatCalico,
		config.registerFromMain,
		ctrl.Log.WithName("pod-network"))
	if err := podNet.Init(); err != nil {
		return err
	}
	podConfigs, err := podNet.List()
	if err != nil {
		return err
	}

	ctx := context.Background()
	for _, c := range podConfigs {
		if err := nodeIPAM.Register(ctx, c.PoolName, c.ContainerId, c.IFace, c.IPv4, c.IPv6); err != nil {
			return err
		}
	}
	if err := nodeIPAM.GC(ctx); err != nil {
		return err
	}

	os.Remove(config.socketPath)
	l, err := net.Listen("unix", config.socketPath)
	if err != nil {
		return err
	}
	server := runners.NewCoildServer(l, mgr, nodeIPAM, podNet, runners.NewNATSetup(config.egressPort), grpcLogger)
	if err := mgr.Add(server); err != nil {
		return err
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		return err
	}

	return nil
}
