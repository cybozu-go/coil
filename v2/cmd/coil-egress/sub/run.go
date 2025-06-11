package sub

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	v2 "github.com/cybozu-go/coil/v2"
	"github.com/cybozu-go/coil/v2/controllers"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/founat"
	egressMetrics "github.com/cybozu-go/coil/v2/pkg/metrics"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	gracefulTimeout = 5 * time.Second
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// +kubebuilder:scaffold:scheme

	metrics.Registry.MustRegister(egressMetrics.NfConnctackCount)
	metrics.Registry.MustRegister(egressMetrics.NfConnctackLimit)
	metrics.Registry.MustRegister(egressMetrics.NfTableMasqueradeBytes)
	metrics.Registry.MustRegister(egressMetrics.NfTableMasqueradePackets)
	metrics.Registry.MustRegister(egressMetrics.NfTableInvalidBytes)
	metrics.Registry.MustRegister(egressMetrics.NfTableInvalidPackets)
}

func subMain() error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&config.zapOpts)))

	myNS := os.Getenv(constants.EnvPodNamespace)
	if myNS == "" {
		return errors.New(constants.EnvPodNamespace + " environment variable must be set")
	}

	myName := os.Getenv(constants.EnvEgressName)
	if myName == "" {
		return errors.New(constants.EnvEgressName + " environment variable must be set")
	}

	myAddresses := strings.Split(os.Getenv(constants.EnvAddresses), ",")
	if len(myAddresses) == 0 {
		return errors.New(constants.EnvAddresses + " environment variable must be set")
	}
	var ipv4, ipv6 net.IP
	protocolMap := make(map[string]struct{})
	for _, addr := range myAddresses {
		n := net.ParseIP(addr)
		if n == nil {
			return errors.New(constants.EnvAddresses + "contains invalid address: " + addr)
		}
		if n4 := n.To4(); n4 != nil {
			ipv4 = n4
			protocolMap["ipv4"] = struct{}{}
		} else {
			ipv6 = n
			protocolMap["ipv6"] = struct{}{}
		}
	}

	protocols := make([]string, 0, 0)
	for protocol, _ := range protocolMap {
		protocols = append(protocols, protocol)
	}

	setupLog.Info("detected local IP addresses", "ipv4", ipv4.String(), "ipv6", ipv6.String())

	timeout := gracefulTimeout
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Metrics: metricsserver.Options{
			BindAddress: config.metricsAddr,
		},
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

	setupLog.Info("initialize FoU tunnel", "port", config.port, "ipv4", ipv4.String(), "ipv6", ipv6.String())
	ft := founat.NewFoUTunnel(config.port, ipv4, ipv6, nil)
	if err := ft.Init(); err != nil {
		return err
	}

	setupLog.Info("initialize Egress", "ipv4", ipv4.String(), "ipv6", ipv6.String())
	eg := founat.NewEgress("eth0", ipv4, ipv6)
	if err := eg.Init(); err != nil {
		return err
	}

	setupLog.Info("setup Pod watcher")
	if err := controllers.SetupPodWatcher(mgr, myNS, myName, ft, config.enableSportAuto, eg, nil); err != nil {
		return err
	}

	setupLog.Info("setup egress metrics collector")
	runner := egressMetrics.NewRunner()
	egressCollector, err := egressMetrics.NewEgressCollector(myNS, os.Getenv("HOSTNAME"), myName, protocols)
	if err != nil {
		return err
	}
	runner.Register(egressCollector)
	go runner.Run(context.Background())

	setupLog.Info("starting manager", "version", v2.Version())
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		return err
	}

	return nil
}
