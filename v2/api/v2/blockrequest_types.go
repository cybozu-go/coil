package v2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BlockRequestSpec defines the desired state of BlockRequest
type BlockRequestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of BlockRequest. Edit BlockRequest_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// BlockRequestStatus defines the observed state of BlockRequest
type BlockRequestStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// BlockRequest is the Schema for the blockrequests API
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
