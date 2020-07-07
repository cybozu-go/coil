package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
)

// AddressBlockReconciler reconciles a AddressBlock object
type AddressBlockReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addressblocks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coil.cybozu.com,resources=addressblocks/status,verbs=get;update;patch

func (r *AddressBlockReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("addressblock", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

func (r *AddressBlockReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&coilv2.AddressBlock{}).
		Complete(r)
}
