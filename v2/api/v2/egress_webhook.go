package v2

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager setups the webhook for Egress
func (r *Egress) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&EgressCustomDefaulter{}).
		WithValidator(&EgressCustomValidator{}).
		Complete()
}

// EgressCustomDefaulter is an empty struct that implements webhook.Defaulter
type EgressCustomDefaulter struct{}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-coil-cybozu-com-v2-egress,mutating=true,failurePolicy=fail,sideEffects=None,groups=coil.cybozu.com,resources=egresses,verbs=create,versions=v2,name=megress.kb.io,admissionReviewVersions={v1,v1beta1}

var _ admission.Defaulter[*Egress] = &EgressCustomDefaulter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *EgressCustomDefaulter) Default(ctx context.Context, egress *Egress) error {
	tmpl := egress.Spec.Template
	if tmpl == nil {
		return nil
	}

	if len(tmpl.Spec.Containers) == 0 {
		tmpl.Spec.Containers = []corev1.Container{
			{
				Name: "egress",
			},
		}
	}
	return nil
}

// EgressCustomValidator is an empty struct that implements webhook.Validator
type EgressCustomValidator struct{}

// +kubebuilder:webhook:path=/validate-coil-cybozu-com-v2-egress,mutating=false,failurePolicy=fail,sideEffects=None,groups=coil.cybozu.com,resources=egresses,verbs=create;update,versions=v2,name=vegress.kb.io,admissionReviewVersions={v1,v1beta1}

var _ admission.Validator[*Egress] = &EgressCustomValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *EgressCustomValidator) ValidateCreate(ctx context.Context, egress *Egress) (warnings admission.Warnings, err error) {
	if errs := egress.Spec.validate(); len(errs) != 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Egress"}, egress.Name, errs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *EgressCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *Egress) (warnings admission.Warnings, err error) {
	if errs := newObj.Spec.validateUpdate(); len(errs) != 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Egress"}, newObj.Name, errs)
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *EgressCustomValidator) ValidateDelete(ctx context.Context, old *Egress) (warnings admission.Warnings, err error) {
	return nil, nil
}
