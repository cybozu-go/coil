package controllers

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
)

// AddressBlockReconciler watches AddressBlocks and notifies a channel
type AddressBlockReconciler struct {
	Notify chan<- struct{}
}

var _ reconcile.Reconciler = &AddressBlockReconciler{}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addressblocks,verbs=get;list;watch

// Reconcile implements Reconciler interface.
func (r *AddressBlockReconciler) Reconcile(ctrl.Request) (ctrl.Result, error) {
	select {
	case r.Notify <- struct{}{}:
	default:
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers this with the manager.
func (r *AddressBlockReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&coilv2.AddressBlock{}).
		Complete(r)
}
