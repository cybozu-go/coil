package v2

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// SetupWebhookWithManager setups the webhook for Egress
func (r *Egress) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-coil-cybozu-com-v2-egress,mutating=true,failurePolicy=fail,groups=coil.cybozu.com,resources=egresses,verbs=create,versions=v2,name=megress.kb.io

var _ webhook.Defaulter = &Egress{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Egress) Default() {
	tmpl := r.Spec.Template
	if tmpl == nil {
		return
	}

	if len(tmpl.Spec.Containers) == 0 {
		tmpl.Spec.Containers = []corev1.Container{
			{
				Name: "egress",
			},
		}
	}
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-coil-cybozu-com-v2-egress,mutating=false,failurePolicy=fail,groups=coil.cybozu.com,resources=egresses,versions=v2,name=vegress.kb.io

var _ webhook.Validator = &Egress{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Egress) ValidateCreate() error {
	errs := r.Spec.validate()
	if len(errs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Egress"}, r.Name, errs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Egress) ValidateUpdate(old runtime.Object) error {
	errs := r.Spec.validateUpdate(old.(*Egress).Spec)
	if len(errs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Egress"}, r.Name, errs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Egress) ValidateDelete() error {
	return nil
}
