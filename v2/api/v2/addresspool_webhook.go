package v2

import (
	"github.com/cybozu-go/coil/v2/pkg/constants"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// SetupWebhookWithManager registers webhooks for AddressPool
func (r *AddressPool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-coil-cybozu-com-v2-addresspool,mutating=true,failurePolicy=fail,sideEffects=None,groups=coil.cybozu.com,resources=addresspools,verbs=create,versions=v2,name=maddresspool.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &AddressPool{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *AddressPool) Default() {
	controllerutil.AddFinalizer(r, constants.FinCoil)
}

// +kubebuilder:webhook:path=/validate-coil-cybozu-com-v2-addresspool,mutating=false,failurePolicy=fail,sideEffects=None,groups=coil.cybozu.com,resources=addresspools,verbs=create;update,versions=v2,name=vaddresspool.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &AddressPool{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *AddressPool) ValidateCreate() error {
	errs := r.Spec.validate()
	if len(errs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "AddressPool"}, r.Name, errs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *AddressPool) ValidateUpdate(old runtime.Object) error {
	errs := r.Spec.validateUpdate(old.(*AddressPool).Spec)
	if len(errs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "AddressPool"}, r.Name, errs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *AddressPool) ValidateDelete() error {
	return nil
}
