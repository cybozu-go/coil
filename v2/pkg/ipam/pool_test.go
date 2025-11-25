package ipam

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
)

var _ = Describe("PoolManager", func() {
	ctx := context.Background()

	AfterEach(func() {
		cleanBlocks()
	})

	Context("default pool", func() {
		It("should allocate blocks", func() {
			pm := NewPoolManager(mgr.GetClient(), mgr.GetAPIReader(), ctrl.Log.WithName("PoolManager"), scheme)

			used, err := pm.IsUsed(ctx, "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(used).To(BeFalse())

			blocks := make([]*coilv2.AddressBlock, 0, 6)
			block, err := pm.AllocateBlock(ctx, "default", "node1", "5a6d130a-adbe-46f9-9da9-bc5da7cc5f04")
			Expect(err).ToNot(HaveOccurred())
			blocks = append(blocks, block)

			Expect(block.Index).To(Equal(int32(0)))
			Expect(block.IPv4).To(Equal(strPtr("10.2.0.0/31")))
			Expect(block.IPv6).To(Equal(strPtr("fd02::200/127")))
			Expect(block.Labels[constants.LabelNode]).To(Equal("node1"))
			Expect(block.Labels[constants.LabelPool]).To(Equal("default"))
			Expect(controllerutil.ContainsFinalizer(block, constants.FinCoil)).To(BeTrue())

			used, err = pm.IsUsed(ctx, "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(used).To(BeTrue())

			verify := &coilv2.AddressBlock{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: block.Name}, verify)
			Expect(err).ToNot(HaveOccurred())

			Expect(promtest.ToFloat64(poolMaxBlocks.WithLabelValues("default"))).To(Equal(float64(6)))
			Expect(promtest.ToFloat64(poolAllocated.WithLabelValues("default"))).To(Equal(float64(1)))

			for i := 0; i < 5; i++ {
				block, err := pm.AllocateBlock(ctx, "default", "node1", "5a6d130a-adbe-46f9-9da9-bc5da7cc5f04")
				Expect(err).ToNot(HaveOccurred())
				blocks = append(blocks, block)
			}

			Expect(promtest.ToFloat64(poolAllocated.WithLabelValues("default"))).To(Equal(float64(6)))

			_, err = pm.AllocateBlock(ctx, "default", "node1", "5a6d130a-adbe-46f9-9da9-bc5da7cc5f04")
			Expect(err).To(MatchError(ErrNoBlock))

			controllerutil.RemoveFinalizer(blocks[4], constants.FinCoil)
			err = k8sClient.Update(ctx, blocks[4])
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Delete(ctx, blocks[4])
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				if err := pm.SyncPool(ctx, "default"); err != nil {
					return err
				}
				if val := int(promtest.ToFloat64(poolAllocated.WithLabelValues("default"))); val != 5 {
					return fmt.Errorf("unexpected allocated_blocks value: %d", val)
				}
				block, err = pm.AllocateBlock(ctx, "default", "node2", "5a6d130a-adbe-46f9-9da9-bc5da7cc5f04")
				return err
			}, 1, 0.1).Should(Succeed())

			Expect(block.Index).To(Equal(int32(4)))
			Expect(block.IPv4).To(Equal(strPtr("10.3.0.0/31")))
			Expect(block.IPv6).To(Equal(strPtr("fd02::300/127")))
			Expect(block.Labels[constants.LabelNode]).To(Equal("node2"))
			Expect(block.Labels[constants.LabelPool]).To(Equal("default"))

			blocks[4] = block
			for _, b := range blocks {
				controllerutil.RemoveFinalizer(b, constants.FinCoil)
				err = k8sClient.Update(ctx, b)
				Expect(err).ToNot(HaveOccurred())
				err = k8sClient.Delete(ctx, b)
				Expect(err).ToNot(HaveOccurred())
			}
			used, err = pm.IsUsed(ctx, "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(used).To(BeFalse())
		})
	})

	Context("IPv4 pool", func() {
		It("should allocate blocks", func() {
			pm := NewPoolManager(mgr.GetClient(), mgr.GetAPIReader(), ctrl.Log.WithName("PoolManager"), scheme)

			blocks := make([]*coilv2.AddressBlock, 0, 2)
			block, err := pm.AllocateBlock(ctx, "v4", "node1", "5a6d130a-adbe-46f9-9da9-bc5da7cc5f04")
			Expect(err).ToNot(HaveOccurred())
			blocks = append(blocks, block)

			Expect(block.Index).To(Equal(int32(0)))
			Expect(block.IPv4).To(Equal(strPtr("10.4.0.0/30")))
			Expect(block.IPv6).To(Equal((*string)(nil)))
			Expect(block.Labels[constants.LabelNode]).To(Equal("node1"))
			Expect(block.Labels[constants.LabelPool]).To(Equal("v4"))

			verify := &coilv2.AddressBlock{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: block.Name}, verify)
			Expect(err).ToNot(HaveOccurred())

			block, err = pm.AllocateBlock(ctx, "v4", "node1", "5a6d130a-adbe-46f9-9da9-bc5da7cc5f04")
			Expect(err).ToNot(HaveOccurred())
			blocks = append(blocks, block)

			_, err = pm.AllocateBlock(ctx, "v4", "node1", "5a6d130a-adbe-46f9-9da9-bc5da7cc5f04")
			Expect(err).To(MatchError(ErrNoBlock))

			controllerutil.RemoveFinalizer(blocks[1], constants.FinCoil)
			err = k8sClient.Update(ctx, blocks[1])
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Delete(ctx, blocks[1])
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				if err := pm.SyncPool(ctx, "v4"); err != nil {
					return err
				}
				block, err = pm.AllocateBlock(ctx, "v4", "node2", "5a6d130a-adbe-46f9-9da9-bc5da7cc5f04")
				return err
			}, 1, 0.1).Should(Succeed())

			Expect(block.Index).To(Equal(int32(1)))
			Expect(block.IPv4).To(Equal(strPtr("10.4.0.4/30")))
			Expect(block.Labels[constants.LabelNode]).To(Equal("node2"))
			Expect(block.Labels[constants.LabelPool]).To(Equal("v4"))
		})
	})
})
