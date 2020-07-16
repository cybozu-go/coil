package v2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BlockRequestSpec defines the desired state of BlockRequest
type BlockRequestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// NodeName is the requesting node name.
	NodeName string `json:"nodeName"`

	// PoolName is the target AddressPool name.
	PoolName string `json:"poolName"`
}

// BlockRequestStatus defines the observed state of BlockRequest
type BlockRequestStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// AddressBlockName is the allocated address block name.
	AddressBlockName string `json:"addressBlockName,omitempty"`

	// Conditions is the list of conditions.
	Conditions []BlockRequestCondition `json:"conditions,omitempty"`
}

// BlockRequestCondition defines the condition of a BlockRequest
type BlockRequestCondition struct {
	// Type of condition, Complete or Failed.
	Type BlockRequestConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition was checked.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// BlockRequestConditionType is to enumerate condition types.
type BlockRequestConditionType string

// Valid values for BlockRequestConditionType
const (
	BlockRequestComplete = "Complete"
	BlockRequestFailed   = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status

// BlockRequest is the Schema for the blockrequests API
//
// The ownerReferences field contains the Node on which coild that created this run.
type BlockRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BlockRequestSpec   `json:"spec,omitempty"`
	Status BlockRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BlockRequestList contains a list of BlockRequest
type BlockRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlockRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BlockRequest{}, &BlockRequestList{})
}
