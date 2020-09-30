package controllers

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"

	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/founat"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// SetupPodWatcher registers pod watching reconciler to mgr.
func SetupPodWatcher(mgr ctrl.Manager, ns, name string, ft founat.FoUTunnel, eg founat.Egress) error {
	r := &podWatcher{
		client:   mgr.GetClient(),
		log:      ctrl.Log.WithName("controllers").WithName("pod-watcher"),
		myNS:     ns,
		myName:   name,
		ft:       ft,
		eg:       eg,
		podAddrs: make(map[string][]net.IP),
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
	client client.Client
	log    logr.Logger
	myNS   string
	myName string
	ft     founat.FoUTunnel
	eg     founat.Egress

	mu       sync.Mutex
	podAddrs map[string][]net.IP
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

func (r *podWatcher) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.log.WithValues("pod", req.NamespacedName)

	pod := &corev1.Pod{}
	err := r.client.Get(ctx, req.NamespacedName, pod)
	if err == nil {
		if !r.shouldHandle(pod) {
			return ctrl.Result{}, err
		}

		if err := r.addPod(pod); err != nil {
			log.Error(err, "failed to setup tunnel")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if apierrors.IsNotFound(err) {
		if err := r.delPod(req.NamespacedName); err != nil {
			log.Error(err, "failed to remove tunnel")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	log.Error(err, "failed to get pod")
	return ctrl.Result{}, nil
}

func (r *podWatcher) addPod(pod *corev1.Pod) error {
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

		link, err := r.ft.AddPeer(ip)
		if errors.Is(err, founat.ErrIPFamilyMismatch) {
			r.log.Info("skipping unsupported pod IP", "pod", pod.Namespace+"/"+pod.Name, "ip", ip.String())
			continue
		}
		if err != nil {
			return err
		}
		if err := r.eg.AddClient(ip, link); err != nil {
			return err
		}
	}

OUTER2:
	for _, eip := range existing {
		for _, ip := range podIPs {
			if eip.Equal(ip) {
				continue OUTER2
			}
		}

		if err := r.ft.DelPeer(eip); err != nil {
			return err
		}
	}

	r.podAddrs[key] = podIPs
	return nil
}

func (r *podWatcher) delPod(n types.NamespacedName) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := n.Namespace + "/" + n.Name
	for _, ip := range r.podAddrs[key] {
		if err := r.ft.DelPeer(ip); err != nil {
			return err
		}
	}

	delete(r.podAddrs, key)
	return nil
}
