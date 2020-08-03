package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
)

// EgressReconciler reconciles a Egress object
type EgressReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=egresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coil.cybozu.com,resources=egresses/status,verbs=get;update;patch

// Reconcile implements Reconciler interface.
// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.1/pkg/reconcile?tab=doc#Reconciler
func (r *EgressReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("egress", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager registers this with the manager.
func (r *EgressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&coilv2.Egress{}).
		Complete(r)
}
