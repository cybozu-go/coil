package controllers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/founat"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	clientPods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: constants.MetricsNS,
			Subsystem: "egress",
			Name:      "client_pod_count",
			Help:      "the number of client pods which use this egress",
		},
		[]string{"namespace", "egress"},
	)
)

func init() {
	metrics.Registry.MustRegister(clientPods)
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// SetupPodWatcher registers pod watching reconciler to mgr.
func SetupPodWatcher(mgr ctrl.Manager, ns, name string, ft founat.FoUTunnel, encapSportAuto bool, eg founat.Egress) error {
	clientPods.Reset()

	r := &podWatcher{
		client:         mgr.GetClient(),
		myNS:           ns,
		myName:         name,
		ft:             ft,
		encapSportAuto: encapSportAuto,
		eg:             eg,
		metric:         clientPods.WithLabelValues(ns, name),
		podAddrs:       make(map[string][]net.IP),
		peers:          make(map[string]map[string]struct{}),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}

// podWatcher adds FoU tunnels for new pods and removes them when pods are deleted.
//
// The mapping between pod and tunnel is kept in memory, so if coil-egress restarts,
// this implementation can leave some tunnels as garbage.  Such garbage tunnels
// do no harm, though.
type podWatcher struct {
	client         client.Client
	myNS           string
	myName         string
	ft             founat.FoUTunnel
	encapSportAuto bool
	eg             founat.Egress
	metric         prometheus.Gauge

	mu       sync.Mutex
	podAddrs map[string][]net.IP
	peers    map[string]map[string]struct{}
}

func (r *podWatcher) shouldHandle(pod *corev1.Pod) bool {
	if pod.Spec.HostNetwork {
		// Egress feature is not available for Pods running in the host network.
		return false
	}

	for k, v := range pod.Annotations {
		if !strings.HasPrefix(k, constants.AnnEgressPrefix) {
			continue
		}

		if k[len(constants.AnnEgressPrefix):] != r.myNS {
			continue
		}

		// shortcut for the most typical case
		if v == r.myName {
			return true
		}

		for _, n := range strings.Split(v, ",") {
			if n == r.myName {
				return true
			}
		}
	}
	return false
}

func isTerminated(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

func (r *podWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pod := &corev1.Pod{}
	err := r.client.Get(ctx, req.NamespacedName, pod)
	if err == nil {
		if !r.shouldHandle(pod) {
			return ctrl.Result{}, err
		}

		if !isTerminated(pod) {
			if err := r.addPod(pod, logger); err != nil {
				logger.Error(err, "failed to setup tunnel")
				return ctrl.Result{}, err
			}
		} else {
			if err := r.delTerminatedPod(pod, logger); err != nil {
				logger.Error(err, "failed to remove tunnel for a terminated pod")
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	if apierrors.IsNotFound(err) {
		if err := r.delPod(req.NamespacedName, logger); err != nil {
			logger.Error(err, "failed to remove tunnel")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	logger.Error(err, "failed to get pod")
	return ctrl.Result{}, nil
}

func (r *podWatcher) addPod(pod *corev1.Pod, logger logr.Logger) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := pod.Namespace + "/" + pod.Name
	existing := r.podAddrs[key]
	podIPs := make([]net.IP, len(pod.Status.PodIPs))
	for i, v := range pod.Status.PodIPs {
		podIPs[i] = net.ParseIP(v.IP)
	}

OUTER:
	for _, ip := range podIPs {
		for _, eip := range existing {
			if ip.Equal(eip) {
				continue OUTER
			}
		}

		link, err := r.ft.AddPeer(ip, r.encapSportAuto)
		if errors.Is(err, founat.ErrIPFamilyMismatch) {
			logger.Info("skipping unsupported pod IP", "pod", pod.Namespace+"/"+pod.Name, "ip", ip.String())
			continue
		}
		if err != nil {
			return err
		}
		if err := r.eg.AddClient(ip, link); err != nil {
			return err
		}
		// link up here
	}

OUTER2:
	for _, eip := range existing {
		for _, ip := range podIPs {
			if eip.Equal(ip) {
				continue OUTER2
			}
		}
		logger.Info("delete peer", "caller", "addPod", "ip", eip.String(), "podIPs", podIPs, "existing", existing)
		if err := r.ft.DelPeer(eip); err != nil {
			return err
		}
	}

	r.podAddrs[key] = podIPs
	for _, ip := range podIPs {
		keySet, ok := r.peers[ip.String()]
		if !ok {
			keySet = map[string]struct{}{}
			r.peers[ip.String()] = keySet
		}
		keySet[key] = struct{}{}
	}

	r.metric.Set(float64(len(r.podAddrs)))
	return nil
}

func (r *podWatcher) delTerminatedPod(pod *corev1.Pod, logger logr.Logger) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.delPeer(pod.Namespace+"/"+pod.Name, "delTerminatedPod", string(pod.Status.Phase), logger)
}

func (r *podWatcher) delPod(n types.NamespacedName, logger logr.Logger) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.delPeer(n.Namespace+"/"+n.Name, "delPod", "", logger)
}

func (r *podWatcher) delPeer(key, caller, podPhase string, logger logr.Logger) error {
	for _, ip := range r.podAddrs[key] {
		existsLivePeers, err := r.existsOtherLivePeers(key, ip.String())
		if err != nil {
			return err
		}
		if !existsLivePeers {
			logger.Info("delete peer", "caller", caller, "ip", ip.String(), "podIPs", r.podAddrs[key], "podPhase", podPhase)
			if err := r.ft.DelPeer(ip); err != nil {
				return err
			}
		}

		if keySet, ok := r.peers[ip.String()]; ok {
			delete(keySet, key)
			if len(keySet) == 0 {
				delete(r.peers, ip.String())
			}
		}
	}

	delete(r.podAddrs, key)
	r.metric.Set(float64(len(r.podAddrs)))
	return nil
}

func (r *podWatcher) existsOtherLivePeers(key, ip string) (bool, error) {
	if keySet, ok := r.peers[ip]; ok {
		if _, ok := keySet[key]; ok {
			return len(keySet) > 1, nil
		}
		return false, fmt.Errorf("keySet in the peers doesn't contain my key. key: %s ip: %s", key, ip)
	}
	return false, fmt.Errorf("peers doesn't contain my IP. key: %s ip: %s", key, ip)
}
