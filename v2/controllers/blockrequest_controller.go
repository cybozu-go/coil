package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
)

// BlockRequestReconciler reconciles a BlockRequest object
type BlockRequestReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=blockrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coil.cybozu.com,resources=blockrequests/status,verbs=get;update;patch

func (r *BlockRequestReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("blockrequest", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

func (r *BlockRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&coilv2.BlockRequest{}).
		Complete(r)
}
