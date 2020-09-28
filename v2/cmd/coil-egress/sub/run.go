package sub

import (
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"github.com/cybozu-go/coil/v2/controllers"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/founat"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	gracefulTimeout = 5 * time.Second
)

func subMain() error {
	ctrl.SetLogger(zap.New(zap.StacktraceLevel(zapcore.DPanicLevel)))

	myNS := os.Getenv(constants.EnvPodNamespace)
	if myNS == "" {
		return errors.New(constants.EnvPodNamespace + " environment variable must be set")
	}

	myName := os.Getenv(constants.EnvPodName)
	if myName == "" {
		return errors.New(constants.EnvPodName + " environment variable must be set")
	}

	myAddresses := strings.Split(os.Getenv(constants.EnvAddresses), ",")
	if len(myAddresses) == 0 {
		return errors.New(constants.EnvAddresses + " environment variable must be set")
	}
	var ipv4, ipv6 net.IP
	for _, addr := range myAddresses {
		n := net.ParseIP(addr)
		if n == nil {
			return errors.New(constants.EnvAddresses + "contains invalid address: " + addr)
		}
		if n4 := n.To4(); n4 != nil {
			ipv4 = n4
		} else {
			ipv6 = n
		}
	}

	log := ctrl.Log.WithName("main")
	log.Info("detected local IP addresses", "ipv4", ipv4.String(), "ipv6", ipv6.String())

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
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

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return err
	}

	ft := founat.NewFoUTunnel(config.port, ipv4, ipv6)
	if err := ft.Init(); err != nil {
		return err
	}

	eg := founat.NewEgress("eth0", ipv4, ipv6)
	if err := eg.Init(); err != nil {
		return err
	}

	if err := controllers.SetupPodWatcher(mgr, myNS, myName, ft, eg); err != nil {
		return err
	}

	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "problem running manager")
		return err
	}

	return nil
}
