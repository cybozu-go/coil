package controllers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/founat"
	"github.com/cybozu-go/coil/v2/pkg/nat"
)

var (
	ClientPods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: constants.MetricsNS,
			Subsystem: "egress",
			Name:      "client_pod_count",
			Help:      "the number of client pods which use this egress",
		},
		[]string{"namespace", "egress"},
	)

	ClientPodInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: constants.MetricsNS,
			Subsystem: "egress",
			Name:      "client_pod_info",
			Help:      "information of a client pod which uses this egress",
		},
		[]string{"namespace", "pod", "pod_ip", "interface", "egress", "egress_namespace"},
	)
)

func init() {
	metrics.Registry.MustRegister(ClientPods)
	metrics.Registry.MustRegister(ClientPodInfo)
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// SetupPodWatcher registers pod watching reconciler to mgr.
func SetupPodWatcher(mgr ctrl.Manager, ns, name string, ft founat.FoUTunnel, encapSportAuto bool, nat nat.Server, cfg *rest.Config) error {
	ClientPods.Reset()
	ClientPodInfo.Reset()

	r := &podWatcher{
		client:         mgr.GetClient(),
		myNS:           ns,
		myName:         name,
		ft:             ft,
		encapSportAuto: encapSportAuto,
		nat:            nat,
		clientPods:     ClientPods.WithLabelValues(ns, name),
		podAddrs:       make(map[string][]net.IP),
		peers:          make(map[string]map[string]struct{}),
	}

	if cfg == nil {
		cfg = config.GetConfigOrDie()
	}

	client, err := client.New(cfg, client.Options{})
	if err != nil {
		return err
	}

	var pods corev1.PodList
	ctx := context.Background()
	if err := client.List(ctx, &pods); err != nil {
		return err
	}

	g := new(errgroup.Group)
	for _, pod := range pods.Items {
		pod := pod
		g.Go(func() error {
			if isTerminated(&pod) {
				return nil
			}
			if !r.shouldHandle(&pod) {
				return nil
			}
			return r.addPod(&pod, log.FromContext(ctx))
		})
	}
	if err := g.Wait(); err != nil {
		return err
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
	nat            nat.Server
	clientPods     prometheus.Gauge

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

	logger.Info("add pod", "pod", pod.Name, "namespace", pod.Namespace)

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
		if err := r.nat.AddClient(ip, link); err != nil {
			return err
		}
		metric := ClientPodInfo.WithLabelValues(pod.GetNamespace(), pod.GetName(), ip.String(), link.Attrs().Name, r.myName, r.myNS)
		metric.Set(1)
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
		if n := ClientPodInfo.DeletePartialMatch(prometheus.Labels{"namespace": pod.GetNamespace(), "pod": pod.GetName(), "pod_ip": eip.String(), "egress": r.myName, "egress_namespace": r.myNS}); n != 1 {
			logger.Error(errors.New("metrics deletion error"), "the number of deleted metrics is not one for", "pod", pod.GetName(), "namespace", pod.GetNamespace())
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

	r.clientPods.Set(float64(len(r.podAddrs)))
	return nil
}

func (r *podWatcher) delTerminatedPod(pod *corev1.Pod, logger logr.Logger) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.delPeer(types.NamespacedName{Namespace: pod.GetNamespace(), Name: pod.GetName()}, "delTerminatedPod", string(pod.Status.Phase), logger)
}

func (r *podWatcher) delPod(n types.NamespacedName, logger logr.Logger) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.delPeer(n, "delPod", "", logger); err != nil {
		return err
	}
	return nil
}

func (r *podWatcher) delPeer(n types.NamespacedName, caller, podPhase string, logger logr.Logger) error {
	key := n.Namespace + "/" + n.Name
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
		if deleted := ClientPodInfo.DeletePartialMatch(prometheus.Labels{"namespace": n.Namespace, "pod": n.Name, "pod_ip": ip.String(), "egress": r.myName, "egress_namespace": r.myNS}); deleted != 1 {
			logger.Error(errors.New("metrics deletion error"), "the number of deleted metrics is not one for", "pod", n.Name, "namespace", n.Namespace)
		}
	}

	delete(r.podAddrs, key)
	r.clientPods.Set(float64(len(r.podAddrs)))
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
