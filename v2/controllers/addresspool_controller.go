package controllers

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/ipam"
)

// AddressPoolReconciler watches child AddressBlocks and pool itself for deletion.
type AddressPoolReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Manager ipam.PoolManager
}

var _ reconcile.Reconciler = &AddressPoolReconciler{}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addresspools,verbs=get;list;watch
// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addressblocks,verbs=get;list;watch

// Reconcile implements Reconciler interface.
// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile?tab=doc#Reconciler
func (r *AddressPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ap := &coilv2.AddressPool{}
	err := r.Client.Get(ctx, req.NamespacedName, ap)

	if apierrors.IsNotFound(err) {
		logger.Info("dropping address pool from manager")
		r.Manager.DropPool(req.Name)
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get address pool: %w", err)
	}

	if err := r.Manager.SyncPool(ctx, req.Name); err != nil {
		return ctrl.Result{}, fmt.Errorf("SyncPool failed: %w", err)
	}

	logger.Info("synchronized")

	if ap.DeletionTimestamp == nil {
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ap, constants.FinCoil) {
		return ctrl.Result{}, nil
	}

	used, err := r.Manager.IsUsed(ctx, req.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("IsUsed failed: %w", err)
	}
	if used {
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(ap, constants.FinCoil)
	if err := r.Update(ctx, ap); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer from address pool: %w", err)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers this with the manager.
func (r *AddressPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&coilv2.AddressPool{}).
		Owns(&coilv2.AddressBlock{}, builder.WithPredicates(predicate.Funcs{
			// predicate.Funcs returns true by default
			CreateFunc: func(event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(event.UpdateEvent) bool {
				return false
			},
			GenericFunc: func(event.GenericEvent) bool {
				return false
			},
		})).
		Complete(r)
}
