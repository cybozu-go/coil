package v2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:JSONPath=`.metadata.labels['coil\.cybozu\.com/node']`,name=Node,type=string
// +kubebuilder:printcolumn:JSONPath=`.metadata.labels['coil\.cybozu\.com/pool']`,name=Pool,type=string
// +kubebuilder:printcolumn:JSONPath=.ipv4,name=IPv4,type=string
// +kubebuilder:printcolumn:JSONPath=.ipv6,name=IPv6,type=string

// AddressBlock is the Schema for the addressblocks API
//
// The ownerReferences field contains the AddressPool where the block is curved from.
type AddressBlock struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Index indicates the index of this block from the origin pool
	// +kubebuilder:validation:Minimum=0
	Index int32 `json:"index"`

	// IPv4 is an IPv4 subnet address
	IPv4 *string `json:"ipv4,omitempty"`

	// IPv6 is an IPv6 subnet address
	IPv6 *string `json:"ipv6,omitempty"`
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
