package runners

import (
	"context"
	"fmt"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("Garbage collector", func() {
	ctx := context.Background()
	var stopCh chan struct{}

	BeforeEach(func() {
		stopCh = make(chan struct{})
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ToNot(HaveOccurred())

		gc := &GarbageCollector{
			Client:    mgr.GetClient(),
			APIReader: mgr.GetAPIReader(),
			Log:       ctrl.Log.WithName("garbage collector"),
			Scheme:    mgr.GetScheme(),
			Interval:  3 * time.Second,
		}
		err = mgr.Add(gc)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			err := mgr.Start(stopCh)
			if err != nil {
				panic(err)
			}
		}()
	})

	AfterEach(func() {
		deleteAllAddressBlocks()
		close(stopCh)
		err := k8sClient.DeleteAllOf(ctx, &coilv2.BlockRequest{})
		Expect(err).To(Succeed())
		time.Sleep(10 * time.Millisecond)
	})

	It("should collect orphaned blocks", func() {
		block := &coilv2.AddressBlock{}
		block.Name = "default-0"
		block.Index = 0
		block.Labels = map[string]string{
			constants.LabelPool: "default",
			constants.LabelNode: "node1",
		}
		err := k8sClient.Create(ctx, block)
		Expect(err).To(Succeed())

		block = &coilv2.AddressBlock{}
		block.Name = "default-1"
		block.Index = 1
		block.Labels = map[string]string{
			constants.LabelPool: "default",
			constants.LabelNode: "node2",
		}
		err = k8sClient.Create(ctx, block)
		Expect(err).To(Succeed())

		block = &coilv2.AddressBlock{}
		block.Name = "v4-0"
		block.Index = 0
		block.Labels = map[string]string{
			constants.LabelPool: "v4",
			constants.LabelNode: "node3",
		}
		err = k8sClient.Create(ctx, block)
		Expect(err).To(Succeed())

		block = &coilv2.AddressBlock{}
		block.Name = "v4-1"
		block.Index = 1
		block.Labels = map[string]string{
			constants.LabelPool: "v4",
			constants.LabelNode: "node1",
		}
		err = k8sClient.Create(ctx, block)
		Expect(err).To(Succeed())

		Eventually(func() error {
			blocks := &coilv2.AddressBlockList{}
			err := k8sClient.List(ctx, blocks)
			if err != nil {
				return err
			}
			count := 0
			for _, b := range blocks.Items {
				if b.Labels[constants.LabelNode] != "node1" {
					return fmt.Errorf("block for node %s still exists", b.Labels[constants.LabelNode])
				}
				count++
			}

			if count != 2 {
				return fmt.Errorf("unexpected count of node1 blocks: %d", count)
			}

			return nil
		}, 5).Should(Succeed())
	})
})
