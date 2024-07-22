package controllers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/founat"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type EgressWatcher struct {
	client.Client
	NodeName   string
	PodNet     nodenet.PodNetwork
	EgressPort int
}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=egresses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// Reconcile implements Reconciler interface.
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *EgressWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	eg := &coilv2.Egress{}
	if err := r.Get(ctx, req.NamespacedName, eg); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get egress")
		return ctrl.Result{}, err
	}
	if eg.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	pods := &corev1.PodList{}
	err := r.Client.List(ctx, pods, client.MatchingFields{
		constants.PodNodeNameKey: r.NodeName,
	})
	if err != nil {
		logger.Error(err, "failed to list Pod")
		return ctrl.Result{}, err
	}

	targetPods := make(map[string]*corev1.Pod)
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.HostNetwork {
			// Pods in host network cannot use egress NAT.
			// So skip it.
			continue
		}
		podIp := pod.Status.PodIP
		// The reconciliation should be triggered only for running pods.
		if pod.Status.Phase == corev1.PodRunning {
			if _, found := targetPods[podIp]; found {
				// multiple running pods have the same address.
				return ctrl.Result{}, fmt.Errorf("multiple pods have the same address: %s", podIp)
			}
			targetPods[podIp] = pod
		}
	}

	for _, targetPod := range targetPods {
		for k, v := range targetPod.Annotations {
			if !strings.HasPrefix(k, constants.AnnEgressPrefix) {
				continue
			}

			if k[len(constants.AnnEgressPrefix):] != eg.Namespace {
				continue
			}

			// shortcut for the most typical case
			if v == eg.Name {
				// Do reconcile
				if err := r.reconcileEgressClient(ctx, eg, targetPod, &logger); err != nil {
					logger.Error(err, "failed to reconcile Egress client pod")
					return ctrl.Result{}, err
				}
				continue
			}

			for _, n := range strings.Split(v, ",") {
				if n == eg.Name {
					if err := r.reconcileEgressClient(ctx, eg, targetPod, &logger); err != nil {
						logger.Error(err, "failed to reconcile Egress client pod")
						return ctrl.Result{}, err
					}
					continue
				}
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *EgressWatcher) reconcileEgressClient(ctx context.Context, eg *coilv2.Egress, pod *corev1.Pod, logger *logr.Logger) error {
	hook, err := r.getHook(ctx, eg, logger)
	if err != nil {
		return fmt.Errorf("failed to setup NAT hook: %w", err)
	}

	var ipv4, ipv6 net.IP
	for _, podIP := range pod.Status.PodIPs {
		ip := net.ParseIP(podIP.IP)
		if ip.To4() != nil {
			ipv4 = ip.To4()
			continue
		}
		if ip.To16() != nil {
			ipv6 = ip.To16()
		}
	}
	if err := r.PodNet.Update(ipv4, ipv6, hook, pod); err != nil {
		return fmt.Errorf("failed to update NAT configuration: %w", err)
	}

	return nil
}

type gwNets struct {
	gateway   net.IP
	networks  []*net.IPNet
	sportAuto bool
}

func (r *EgressWatcher) getHook(ctx context.Context, eg *coilv2.Egress, logger *logr.Logger) (nodenet.SetupHook, error) {
	var gw gwNets
	svc := &corev1.Service{}

	if err := r.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, svc); err != nil {
		return nil, err
	}

	// See getHook in coild_server.go
	svcIP := net.ParseIP(svc.Spec.ClusterIP)
	if svcIP == nil {
		return nil, fmt.Errorf("invalid ClusterIP in Service %s %s", eg.Name, svc.Spec.ClusterIP)
	}
	var subnets []*net.IPNet
	if ip4 := svcIP.To4(); ip4 != nil {
		svcIP = ip4
		for _, sn := range eg.Spec.Destinations {
			_, subnet, err := net.ParseCIDR(sn)
			if err != nil {
				return nil, fmt.Errorf("invalid network in Egress %s", eg.Name)
			}
			if subnet.IP.To4() != nil {
				subnets = append(subnets, subnet)
			}
		}
	} else {
		for _, sn := range eg.Spec.Destinations {
			_, subnet, err := net.ParseCIDR(sn)
			if err != nil {
				return nil, fmt.Errorf("invalid network in Egress %s", eg.Name)
			}
			if subnet.IP.To4() == nil {
				subnets = append(subnets, subnet)
			}
		}
	}

	if len(subnets) > 0 {
		gw = gwNets{gateway: svcIP, networks: subnets, sportAuto: eg.Spec.FouSourcePortAuto}
		return r.hook(gw, logger), nil
	}

	return nil, nil
}

func (r *EgressWatcher) hook(gwn gwNets, log *logr.Logger) func(ipv4, ipv6 net.IP) error {
	return func(ipv4, ipv6 net.IP) error {
		// We assume that coild already has configured NAT for the client,
		// so we ensure that both FoUTunnel and NATClient have been initialized.
		ft := founat.NewFoUTunnel(r.EgressPort, ipv4, ipv6, func(message string) {
			log.Info(message)
		})
		if !ft.IsInitialized() {
			return errors.New("fouTunnel hasn't been initialized")
		}
		cl := founat.NewNatClient(ipv4, ipv6, nil, func(message string) {
			log.Info(message)
		})
		initialized, err := cl.IsInitialized()
		if !initialized {
			return fmt.Errorf("natClient hasn't been initialized: %w", err)
		}

		link, err := ft.AddPeer(gwn.gateway, gwn.sportAuto)
		if errors.Is(err, founat.ErrIPFamilyMismatch) {
			// ignore unsupported IP family link
			log.Info("ignored unsupported gateway", "gw", gwn.gateway)
			return nil
		}
		if err != nil {
			return err
		}
		if err := cl.AddEgress(link, gwn.networks); err != nil {
			return err
		}

		return nil
	}
}

// SetupWithManager registers this with the manager.
func (r *EgressWatcher) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&coilv2.Egress{}).
		Complete(r)
}
