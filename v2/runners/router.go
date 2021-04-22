package runners

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	initOnce   sync.Once
	syncCount  prometheus.Counter
	routeGauge prometheus.Gauge
)

func initMetrics(nodeName string) {
	initOnce.Do(func() {
		syncCount = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   constants.MetricsNS,
			Subsystem:   "router",
			Name:        "syncs_total",
			Help:        "Number of times coil-router has synchronized routes",
			ConstLabels: prometheus.Labels{"node": nodeName},
		})
		metrics.Registry.MustRegister(syncCount)

		routeGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   constants.MetricsNS,
			Subsystem:   "router",
			Name:        "routes_synced",
			Help:        "Number of routes synchronized to kernel",
			ConstLabels: prometheus.Labels{"node": nodeName},
		})

		metrics.Registry.MustRegister(routeGauge)
	})
}

// NewRouter creates a manager.Runnable for coil-router.
func NewRouter(mgr manager.Manager, log logr.Logger, nodeName string, notifyCh <-chan struct{}, syncer nodenet.RouteSyncer, interval time.Duration) manager.Runnable {
	return &router{
		Client:    mgr.GetClient(),
		apiReader: mgr.GetAPIReader(),
		log:       log,
		nodeName:  nodeName,
		notifyCh:  notifyCh,
		syncer:    syncer,
		interval:  interval,
	}
}

type router struct {
	client.Client
	apiReader client.Reader
	log       logr.Logger
	nodeName  string
	notifyCh  <-chan struct{}
	syncer    nodenet.RouteSyncer
	interval  time.Duration
}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addressblocks,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list

var _ manager.LeaderElectionRunnable = &router{}

// NeedLeaderElection implements manager.LeaderElectionRunnable
func (r *router) NeedLeaderElection() bool {
	return false
}

func (r *router) Start(ctx context.Context) error {
	initMetrics(r.nodeName)

	tick := time.NewTicker(r.interval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-r.notifyCh:
		case <-tick.C:
		}
		if err := r.sync(context.Background()); err != nil {
			r.log.Error(err, "synchronizing block information failed")
			return err
		}
		syncCount.Add(1)
	}
}

type nodeIP struct {
	IPv4 net.IP
	IPv6 net.IP
}

func (r *router) sync(ctx context.Context) error {
	nodes := &corev1.NodeList{}
	if err := r.apiReader.List(ctx, nodes); err != nil {
		return fmt.Errorf("failed to list Nodes: %w", err)
	}
	nodeMap := make(map[string]nodeIP)
	for _, n := range nodes.Items {
		if n.Name == r.nodeName {
			// ignore the running node
			continue
		}

		var ipv4, ipv6 net.IP
		for _, a := range n.Status.Addresses {
			if a.Type != corev1.NodeInternalIP {
				continue
			}
			ip := net.ParseIP(a.Address)
			if ip.To4() != nil {
				ipv4 = ip.To4()
				continue
			}
			if ip.To16() != nil {
				ipv6 = ip.To16()
			}
		}
		nodeMap[n.Name] = nodeIP{IPv4: ipv4, IPv6: ipv6}
	}

	blocks := &coilv2.AddressBlockList{}
	if err := r.Client.List(ctx, blocks); err != nil {
		return fmt.Errorf("failed to list AddressBlocks: %w", err)
	}
	nRoutes := 0
	giMap := make(map[string]*nodenet.GatewayInfo)
	for _, b := range blocks.Items {
		nodeName := b.Labels[constants.LabelNode]
		nm, ok := nodeMap[nodeName]
		if !ok {
			// node might be deleted
			continue
		}

		if b.IPv4 != nil {
			_, n, _ := net.ParseCIDR(*b.IPv4)
			gw := nm.IPv4
			if gw == nil {
				r.log.Info("node has no IPv4 address", "node", nodeName)
				goto IPv6
			}

			nRoutes++
			gwStr := gw.String()
			if gi, ok := giMap[gwStr]; ok {
				gi.Networks = append(gi.Networks, n)
			} else {
				giMap[gwStr] = &nodenet.GatewayInfo{
					Gateway:  gw,
					Networks: []*net.IPNet{n},
				}
			}
		}

	IPv6:
		if b.IPv6 != nil {
			_, n, _ := net.ParseCIDR(*b.IPv6)
			gw := nm.IPv6
			if gw == nil {
				r.log.Info("node has no IPv6 address", "node", nodeName)
				continue
			}

			nRoutes++
			gwStr := gw.String()
			if gi, ok := giMap[gwStr]; ok {
				gi.Networks = append(gi.Networks, n)
			} else {
				giMap[gwStr] = &nodenet.GatewayInfo{
					Gateway:  gw,
					Networks: []*net.IPNet{n},
				}
			}
		}
	}

	routeGauge.Set(float64(nRoutes))

	gis := make([]nodenet.GatewayInfo, 0, len(giMap))
	for _, gi := range giMap {
		gis = append(gis, *gi)
	}

	return r.syncer.Sync(gis)
}
