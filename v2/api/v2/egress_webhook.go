package v2

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager setups the webhook for Egress
func (r *Egress) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-coil-cybozu-com-v2-egress,mutating=true,failurePolicy=fail,sideEffects=None,groups=coil.cybozu.com,resources=egresses,verbs=create,versions=v2,name=megress.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomDefaulter = &Egress{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Egress) Default(ctx context.Context, obj runtime.Object) error {
	tmpl := r.Spec.Template
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

// +kubebuilder:webhook:path=/validate-coil-cybozu-com-v2-egress,mutating=false,failurePolicy=fail,sideEffects=None,groups=coil.cybozu.com,resources=egresses,verbs=create;update,versions=v2,name=vegress.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomValidator = &Egress{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Egress) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	errs := r.Spec.validate()
	if len(errs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Egress"}, r.Name, errs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Egress) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (warnings admission.Warnings, err error) {
	errs := r.Spec.validateUpdate()
	if len(errs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Egress"}, r.Name, errs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Egress) ValidateDelete(ctx context.Context, old runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}
