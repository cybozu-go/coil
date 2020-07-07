package v2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AddressBlockSpec defines the desired state of AddressBlock
type AddressBlockSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of AddressBlock. Edit AddressBlock_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// AddressBlockStatus defines the observed state of AddressBlock
type AddressBlockStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// AddressBlock is the Schema for the addressblocks API
type AddressBlock struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddressBlockSpec   `json:"spec,omitempty"`
	Status AddressBlockStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AddressBlockList contains a list of AddressBlock
type AddressBlockList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressBlock `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddressBlock{}, &AddressBlockList{})
}
