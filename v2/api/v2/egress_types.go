package v2

import (
	"net"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EgressSpec defines the desired state of Egress
type EgressSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Destinations is a list of IP networks in CIDR format.
	// +kubebuilder:validation:MinItems=1
	Destinations []string `json:"destinations"`

	// Replicas is the desired number of egress (SNAT) pods.
	// Defaults to 1.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas int32 `json:"replicas"`

	// Strategy describes how to replace existing pods with new ones.
	// Ref. https://pkg.go.dev/k8s.io/api/apps/v1?tab=doc#DeploymentStrategy
	// +optional
	Strategy *appsv1.DeploymentStrategy `json:"strategy,omitempty"`

	// Template is an optional template for egress pods.
	// A container named "egress" is special.  It is the main container of
	// egress pods and usually is not meant to be modified.
	// +optional
	Template *EgressPodTemplate `json:"template,omitempty"`

	// SessionAffinity is to specify the same field of Service for the Egress.
	// However, the default is changed from None to ClientIP.
	// Ref. https://pkg.go.dev/k8s.io/api/core/v1?tab=doc#ServiceSpec
	// +kubebuilder:validation:Enum=ClientIP;None
	// +kubebuilder:default=ClientIP
	// +optional
	SessionAffinity corev1.ServiceAffinity `json:"sessionAffinity,omitempty"`

	// SessionAffinityConfig is to specify the same field of Service for Egress.
	// Ref. https://pkg.go.dev/k8s.io/api/core/v1?tab=doc#ServiceSpec
	// +optional
	SessionAffinityConfig *corev1.SessionAffinityConfig `json:"sessionAffinityConfig,omitempty"`

	// FouSourcePortAuto indicates that the source port number in foo-over-udp encapsulation
	// should be chosen automatically.
	// If set to true, the kernel picks a flow based on the flow hash of the encapsulated packet.
	// The default is false.
	// +optional
	FouSourcePortAuto bool `json:"fouSourcePortAuto,omitempty"`

	// PodDisruptionBudget is an optional PodDisruptionBudget for Egress NAT pods.
	// +optional
	PodDisruptionBudget *EgressPDBSpec `json:"podDisruptionBudget,omitempty"`

	// OriginatingOnly indicates connections that should use egress interface.
	// If set to true, only outgoing connections that originate in the egress client will use egress.
	// The default is false.
	// +optional
	OriginatingOnly bool `json:"originatingOnly,omitempty"`
}

// EgressPodTemplate defines pod template for Egress
//
// This is almost the same as corev1.PodTemplate but is simplified to
// workaround JSON patch issues.
type EgressPodTemplate struct {
	// Metadata defines optional labels and annotations
	// +optional
	Metadata `json:"metadata,omitempty"`

	// Spec defines the pod template spec.
	// +optional
	Spec corev1.PodSpec `json:"spec,omitempty"`
}

// EgressPDB defines PDB for Egress
type EgressPDBSpec struct {
	// MinAvailable is the minimum number of pods that must be available at any given time.
	// +optional
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`

	// MaxUnavailable is the maximum number of pods that can be unavailable at any given time.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// Metadata defines a simplified version of ObjectMeta.
type Metadata struct {
	// Annotations are optional annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels are optional labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

func (es EgressSpec) validate() field.ErrorList {
	var allErrs field.ErrorList
	p := field.NewPath("spec")

	pp := p.Child("destinations")
	for i, na := range es.Destinations {
		_, _, err := net.ParseCIDR(na)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(pp.Index(i), na, err.Error()))
		}
	}

	if es.Strategy != nil {
		switch es.Strategy.Type {
		case appsv1.RecreateDeploymentStrategyType:
		case appsv1.RollingUpdateDeploymentStrategyType:
		default:
			allErrs = append(allErrs, field.NotSupported(p.Child("strategy", "type"), es.Strategy.Type, []string{
				string(appsv1.RecreateDeploymentStrategyType),
				string(appsv1.RollingUpdateDeploymentStrategyType),
			}))
		}
	}

	if es.Template != nil {
		pp := p.Child("template", "metadata")
		allErrs = append(allErrs, validation.ValidateLabels(es.Template.Labels, pp.Child("labels"))...)
		pp = pp.Child("annotations")
		for k := range es.Template.Annotations {
			allErrs = append(allErrs, validation.ValidateLabelName(k, pp)...)
		}
	}

	if es.PodDisruptionBudget != nil {
		pp := p.Child("podDisruptionBudget")
		allErrs = append(allErrs, validatePodDisruptionBudget(*es.PodDisruptionBudget, pp)...)
	}

	return allErrs
}

func (es EgressSpec) validateUpdate() field.ErrorList {
	return es.validate()
}

// For validation of PodDisruptionBudget
// Ref. https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/policy/validation/validation.go
func validatePodDisruptionBudget(pdb EgressPDBSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if pdb.MinAvailable != nil && pdb.MaxUnavailable != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, pdb, "minAvailable and maxUnavailable cannot be both set"))
	}

	if pdb.MinAvailable != nil {
		allErrs = append(allErrs, validatePositiveIntOrPercent(*pdb.MinAvailable, fldPath.Child("minAvailable"))...)
		allErrs = append(allErrs, isNotMoreThan100Percent(*pdb.MinAvailable, fldPath.Child("minAvailable"))...)
	}

	if pdb.MaxUnavailable != nil {
		allErrs = append(allErrs, validatePositiveIntOrPercent(*pdb.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
		allErrs = append(allErrs, isNotMoreThan100Percent(*pdb.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
	}

	return allErrs
}

func validatePositiveIntOrPercent(intOrPercent intstr.IntOrString, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	switch intOrPercent.Type {
	case intstr.String:
		for _, msg := range utilvalidation.IsValidPercent(intOrPercent.StrVal) {
			allErrs = append(allErrs, field.Invalid(fldPath, intOrPercent, msg))
		}
	case intstr.Int:
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(intOrPercent.IntValue()), fldPath)...)
	default:
		allErrs = append(allErrs, field.Invalid(fldPath, intOrPercent, "must be an integer or percentage (e.g '5%%')"))
	}
	return allErrs
}

func isNotMoreThan100Percent(intOrStringValue intstr.IntOrString, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	value, isPercent := getPercentValue(intOrStringValue)
	if !isPercent || value <= 100 {
		return nil
	}
	allErrs = append(allErrs, field.Invalid(fldPath, intOrStringValue, "must not be greater than 100%"))
	return allErrs
}

func getPercentValue(intOrStringValue intstr.IntOrString) (int, bool) {
	if intOrStringValue.Type != intstr.String {
		return 0, false
	}
	if len(utilvalidation.IsValidPercent(intOrStringValue.StrVal)) != 0 {
		return 0, false
	}
	value, _ := strconv.Atoi(intOrStringValue.StrVal[:len(intOrStringValue.StrVal)-1])
	return value, true
}

// EgressStatus defines the observed state of Egress
type EgressStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Replicas is copied from the underlying Deployment's status.replicas.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Selector is a serialized label selector in string form.
	Selector string `json:"selector,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:selectorpath=.status.selector,specpath=.spec.replicas,statuspath=.status.replicas

// Egress is the Schema for the egresses API
type Egress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EgressSpec   `json:"spec,omitempty"`
	Status EgressStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EgressList contains a list of Egress
type EgressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Egress `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Egress{}, &EgressList{})
}
