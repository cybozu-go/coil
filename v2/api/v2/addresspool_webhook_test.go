package v2

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AddressPool Webhook", func() {
	ctx := context.TODO()

	BeforeEach(func() {
		r := &AddressPool{}
		r.Name = "test"
		err := k8sClient.Delete(ctx, r)
		if err == nil {
			return
		}
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("should create an address pool with sane defaults", func() {
		r := &unstructured.Unstructured{}
		r.SetGroupVersionKind(GroupVersion.WithKind("AddressPool"))
		r.SetName("test")
		r.UnstructuredContent()["spec"] = map[string]interface{}{
			"subnets": []interface{}{
				map[string]interface{}{
					"ipv4": "10.2.0.0/24",
				},
			},
		}

		err := k8sClient.Create(ctx, r)
		Expect(err).NotTo(HaveOccurred())

		ap := &AddressPool{}
		err = k8sClient.Get(ctx, client.ObjectKey{Name: "test"}, ap)
		Expect(err).NotTo(HaveOccurred())
		Expect(ap.Spec.BlockSizeBits).To(BeNumerically("==", 5))
	})

	It("should create an address pool of blockSizeBits=0", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				Subnets: []SubnetSet{makeSubnetSet("10.2.0.0/24", "")},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.Spec.BlockSizeBits).To(BeNumerically("==", 0))
	})

	It("should allow a valid IPv4 address pool", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: 2,
				Subnets:       []SubnetSet{makeSubnetSet("10.2.0.0/24", "")},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should deny invalid subnet", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: -1,
				Subnets:       []SubnetSet{makeSubnetSet("a.b.c.d/24", "")},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).To(HaveOccurred())
	})

	It("should deny invalid block size", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: -1,
				Subnets:       []SubnetSet{makeSubnetSet("10.2.0.0/24", "")},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).To(HaveOccurred())
	})

	It("should deny empty subnets", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: 2,
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).To(HaveOccurred())
	})

	It("should deny too small subnets", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: 8,
				Subnets:       []SubnetSet{makeSubnetSet("10.2.0.0/30", "")},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).To(HaveOccurred())
	})

	It("should allow appending new subnets", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: 0,
				Subnets:       []SubnetSet{makeSubnetSet("", "fd02::/120")},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).NotTo(HaveOccurred())

		r.Spec.Subnets = append(r.Spec.Subnets, makeSubnetSet("", "fd03::/112"))
		err = k8sClient.Update(ctx, r)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should allow appending too small subnets", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: 8,
				Subnets:       []SubnetSet{makeSubnetSet("", "fd02::/120")},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).NotTo(HaveOccurred())

		r.Spec.Subnets = append(r.Spec.Subnets, makeSubnetSet("", "fd03::/124"))
		err = k8sClient.Update(ctx, r)
		Expect(err).To(HaveOccurred())
	})

	It("should deny changing block size", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: 2,
				Subnets:       []SubnetSet{makeSubnetSet("10.2.0.0/24", "")},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).NotTo(HaveOccurred())

		r.Spec.BlockSizeBits = 4
		err = k8sClient.Update(ctx, r)
		Expect(err).To(HaveOccurred())
	})

	It("should deny removing subnets", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: 2,
				Subnets: []SubnetSet{
					makeSubnetSet("10.2.0.0/24", ""),
					makeSubnetSet("10.3.0.0/24", ""),
				},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).NotTo(HaveOccurred())

		r.Spec.Subnets = r.Spec.Subnets[:1]
		err = k8sClient.Update(ctx, r)
		Expect(err).To(HaveOccurred())
	})

	It("should deny changing subnets", func() {
		r := &AddressPool{
			Spec: AddressPoolSpec{
				BlockSizeBits: 2,
				Subnets: []SubnetSet{
					makeSubnetSet("10.2.0.0/24", ""),
					makeSubnetSet("10.3.0.0/24", ""),
				},
			},
		}
		r.Name = "test"

		err := k8sClient.Create(ctx, r)
		Expect(err).NotTo(HaveOccurred())

		r.Spec.Subnets[1] = makeSubnetSet("10.4.0.0/24", "")
		err = k8sClient.Update(ctx, r)
		Expect(err).To(HaveOccurred())
	})
})
