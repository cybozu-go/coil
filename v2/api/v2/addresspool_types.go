package v2

import (
	"errors"
	"net"

	"github.com/cybozu-go/coil/v2/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SubnetSet defines a IPv4-only or IPv6-only or IPv4/v6 dual stack subnet
// A dual stack subnet must has the same size subnet of IPv4 and IPv6.
type SubnetSet struct {
	// IPv4 is an IPv4 subnet like "10.2.0.0/16"
	IPv4 *string `json:"ipv4,omitempty"`

	// IPv6 is an IPv6 subnet like "fd00:0200::/112"
	IPv6 *string `json:"ipv6,omitempty"`
}

// Validate validates the subnet set
func (ss SubnetSet) Validate(minSize int) error {
	if ss.IPv4 == nil && ss.IPv6 == nil {
		return errors.New("invalid subnet")
	}

	var v4Size, v6Size int
	if ss.IPv4 != nil {
		_, n, err := net.ParseCIDR(*ss.IPv4)
		if err != nil {
			return err
		}
		ones, bits := n.Mask.Size()
		if bits != 32 {
			return errors.New("invalid IPv4 subnet: " + *ss.IPv4)
		}
		v4Size = bits - ones
		if v4Size < minSize {
			return errors.New("too narrow subnet: " + *ss.IPv4)
		}
	}

	if ss.IPv6 != nil {
		_, n, err := net.ParseCIDR(*ss.IPv6)
		if err != nil {
			return err
		}
		ones, bits := n.Mask.Size()
		if bits != 128 {
			return errors.New("invalid IPv6 subnet: " + *ss.IPv6)
		}
		v6Size = bits - ones
		if v6Size < minSize {
			return errors.New("too narrow subnet: " + *ss.IPv6)
		}
	}

	if ss.IPv4 != nil && ss.IPv6 != nil && v4Size != v6Size {
		return errors.New("dual stack subnet must be the same size")
	}

	return nil
}

// IsIPv4 returns true if ss represents an IPv4-only subnet
func (ss SubnetSet) IsIPv4() bool {
	return ss.IPv4 != nil && ss.IPv6 == nil
}

// IsIPv6 returns true if ss represents an IPv6-only subnet
func (ss SubnetSet) IsIPv6() bool {
	return ss.IPv4 == nil && ss.IPv6 != nil
}

// IsDualStack returns true if ss represents IPv4/v6 dual stack subnet
func (ss SubnetSet) IsDualStack() bool {
	return ss.IPv4 != nil && ss.IPv6 != nil
}

// Equal returns true if ss equals to x
func (ss SubnetSet) Equal(x SubnetSet) bool {
	switch {
	case ss.IPv4 != nil:
		if x.IPv4 == nil {
			return false
		}
		if *ss.IPv4 != *x.IPv4 {
			return false
		}
	case x.IPv4 != nil:
		return false
	}

	switch {
	case ss.IPv6 != nil:
		if x.IPv6 == nil {
			return false
		}
		if *ss.IPv6 != *x.IPv6 {
			return false
		}
	case x.IPv6 != nil:
		return false
	}

	return true
}

// GetBlock curves Nth block from the pool
func (ss SubnetSet) GetBlock(n uint, sizeBits int) (ipv4 *net.IPNet, ipv6 *net.IPNet) {
	blockOffset := (int64(1) << sizeBits) * int64(n)
	if ss.IPv4 != nil {
		_, n, err := net.ParseCIDR(*ss.IPv4)
		if err != nil {
			panic(err)
		}

		ipv4 = &net.IPNet{
			IP:   util.IPAdd(n.IP, blockOffset),
			Mask: net.CIDRMask(32-sizeBits, 32),
		}
		if ipv4.IP == nil {
			panic("bug")
		}
	}
	if ss.IPv6 != nil {
		_, n, err := net.ParseCIDR(*ss.IPv6)
		if err != nil {
			panic(err)
		}

		ipv6 = &net.IPNet{
			IP:   util.IPAdd(n.IP, blockOffset),
			Mask: net.CIDRMask(128-sizeBits, 128),
		}
		if ipv6.IP == nil {
			panic("bug")
		}
	}

	return
}

// AddressPoolSpec defines the desired state of AddressPool
type AddressPoolSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// BlockSizeBits specifies the size of the address blocks curved from this pool.
	// If this is 5, a block will have 2^5 = 32 addresses.  Default is 5.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=0
	// +optional
	BlockSizeBits int32 `json:"blockSizeBits"`

	// Subnets is a list of IPv4, or IPv6, or dual stack IPv4/IPv6 subnets in this pool.
	// All items in the list should be consistent to have the same set of subnets.
	// For example, if the first item is an IPv4 subnet, the other items must also be
	// an IPv4 subnet.
	//
	// This field can be updated only by adding subnets to the list.
	// +kubebuilder:validation:MinItems=1
	Subnets []SubnetSet `json:"subnets"`
}

func (aps AddressPoolSpec) validate() field.ErrorList {
	var allErrs field.ErrorList
	p := field.NewPath("spec", "subnets")
	for i, n := range aps.Subnets {
		if err := n.Validate(int(aps.BlockSizeBits)); err != nil {
			allErrs = append(allErrs, field.Invalid(p.Index(i), "", err.Error()))
		}
	}

	return allErrs
}

func (aps AddressPoolSpec) validateUpdate(old AddressPoolSpec) field.ErrorList {
	var allErrs field.ErrorList
	p := field.NewPath("spec")

	if aps.BlockSizeBits != old.BlockSizeBits {
		allErrs = append(allErrs, field.Forbidden(p.Child("blockSizeBits"), "unchangeable"))
	}

	p = p.Child("subnets")
	if len(old.Subnets) > len(aps.Subnets) {
		allErrs = append(allErrs, field.Forbidden(p, "unshrinkable"))
	} else {
		for i := 0; i < len(old.Subnets); i++ {
			if !aps.Subnets[i].Equal(old.Subnets[i]) {
				allErrs = append(allErrs, field.Forbidden(p.Index(i), "unchangeable"))
			}
		}

		for i := len(old.Subnets); i < len(aps.Subnets); i++ {
			if err := aps.Subnets[i].Validate(int(aps.BlockSizeBits)); err != nil {
				allErrs = append(allErrs, field.Invalid(p.Index(i), "", err.Error()))
			}
		}
	}

	return allErrs
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:JSONPath=.spec.blockSizeBits,name="BlockSize Bits",type=integer

// AddressPool is the Schema for the addresspools API
type AddressPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AddressPoolSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// AddressPoolList contains a list of AddressPool
type AddressPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddressPool{}, &AddressPoolList{})
}
