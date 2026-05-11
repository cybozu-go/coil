package v2

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/cybozu-go/coil/v2/pkg/constants"
)

// SetupWebhookWithManager registers webhooks for AddressPool
func (r *AddressPool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&AddressPoolCustomDefaulter{}).
		WithValidator(&AddressPoolCustomValidator{}).
		Complete()
}

// AddressPoolCustomDefaulter is an empty struct that implements webhook.Defaulter
type AddressPoolCustomDefaulter struct{}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-coil-cybozu-com-v2-addresspool,mutating=true,failurePolicy=fail,sideEffects=None,groups=coil.cybozu.com,resources=addresspools,verbs=create,versions=v2,name=maddresspool.kb.io,admissionReviewVersions={v1,v1beta1}

var _ admission.Defaulter[*AddressPool] = &AddressPoolCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *AddressPoolCustomDefaulter) Default(ctx context.Context, addressPool *AddressPool) error {
	controllerutil.AddFinalizer(addressPool, constants.FinCoil)
	return nil
}

// AddressPoolCustomValidator is an empty struct that implements webhook.Validator
type AddressPoolCustomValidator struct{}

// +kubebuilder:webhook:path=/validate-coil-cybozu-com-v2-addresspool,mutating=false,failurePolicy=fail,sideEffects=None,groups=coil.cybozu.com,resources=addresspools,verbs=create;update,versions=v2,name=vaddresspool.kb.io,admissionReviewVersions={v1,v1beta1}

var _ admission.Validator[*AddressPool] = &AddressPoolCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *AddressPoolCustomValidator) ValidateCreate(ctx context.Context, addressPool *AddressPool) (warnings admission.Warnings, err error) {
	if errs := addressPool.Spec.validate(); len(errs) != 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "AddressPool"}, addressPool.Name, errs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *AddressPoolCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *AddressPool) (warnings admission.Warnings, err error) {
	if errs := newObj.Spec.validateUpdate(oldObj.Spec); len(errs) != 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "AddressPool"}, newObj.Name, errs)
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *AddressPoolCustomValidator) ValidateDelete(ctx context.Context, obj *AddressPool) (warnings admission.Warnings, err error) {
	return nil, nil
}
