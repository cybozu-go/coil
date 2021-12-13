package controllers

import (
	"context"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/indexing"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BlockRequest reconciler", func() {
	ctx := context.Background()
	var cancel context.CancelFunc
	var poolMgr *mockPoolManager

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.TODO())

		br := &coilv2.BlockRequest{}
		br.Name = "br-0"
		br.Spec.NodeName = "node0"
		br.Spec.PoolName = "default"
		err := k8sClient.Create(ctx, br)
		Expect(err).To(Succeed())

		br = &coilv2.BlockRequest{}
		br.Name = "br-1"
		br.Spec.NodeName = "node1"
		br.Spec.PoolName = "default"
		err = k8sClient.Create(ctx, br)
		Expect(err).To(Succeed())
		br.Status.Conditions = []coilv2.BlockRequestCondition{
			{
				Type:               coilv2.BlockRequestComplete,
				Status:             corev1.ConditionTrue,
				LastProbeTime:      metav1.Now(),
				LastTransitionTime: metav1.Now(),
				Reason:             "foo",
			},
		}
		err = k8sClient.Status().Update(ctx, br)
		Expect(err).To(Succeed())

		poolMgr = &mockPoolManager{}
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(indexing.SetupIndexForAddressBlock(ctx, mgr)).ToNot(HaveOccurred())

		brr := &BlockRequestReconciler{
			Client:  mgr.GetClient(),
			Manager: poolMgr,
			Scheme:  mgr.GetScheme(),
		}
		err = brr.SetupWithManager(mgr)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			err := mgr.Start(ctx)
			if err != nil {
				panic(err)
			}
		}()
		time.Sleep(100 * time.Millisecond)
	})

	AfterEach(func() {
		cancel()
		err := k8sClient.DeleteAllOf(context.Background(), &coilv2.BlockRequest{})
		Expect(err).To(Succeed())
		err = k8sClient.DeleteAllOf(context.Background(), &coilv2.AddressBlock{})
		Expect(err).To(Succeed())
		time.Sleep(10 * time.Millisecond)
	})

	It("works as expected", func() {
		By("checking that it reconciles unhandled request at the startup")
		Eventually(func() int {
			return poolMgr.GetAllocated()
		}).Should(Equal(1))

		By("checking that it does not allocate new block for the request that a block has already been assigned")
		Eventually(func() int {
			br := &coilv2.BlockRequest{}
			k8sClient.Get(ctx, client.ObjectKey{Name: "br-0"}, br)
			return len(br.Status.Conditions)
		}).Should(Equal(1))

		br := &coilv2.BlockRequest{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: "br-0"}, br)
		Expect(err).To(Succeed())

		ab := &coilv2.AddressBlock{}
		ab.Name = "default-0-already-allocated"
		ab.Labels = map[string]string{
			constants.LabelPool:    "default",
			constants.LabelNode:    "node0",
			constants.LabelRequest: string(br.UID),
		}
		err = k8sClient.Create(ctx, ab)
		Expect(err).To(Succeed())

		br.Status.Conditions = []coilv2.BlockRequestCondition{}
		err = k8sClient.Status().Update(ctx, br)
		Expect(err).To(Succeed())

		Eventually(func() int {
			br := &coilv2.BlockRequest{}
			k8sClient.Get(ctx, client.ObjectKey{Name: "br-0"}, br)
			return len(br.Status.Conditions)
		}).Should(Equal(1))

		Eventually(func() int {
			return poolMgr.GetAllocated()
		}).Should(Equal(1))

		By("checking that it can reconcile quickly upon a new request")
		br = &coilv2.BlockRequest{}
		br.Name = "br-2"
		br.Spec.NodeName = "node2"
		br.Spec.PoolName = "default"
		err = k8sClient.Create(ctx, br)
		Expect(err).To(Succeed())

		Eventually(func() int {
			return poolMgr.GetAllocated()
		}).Should(Equal(2))

		Eventually(func() string {
			br := &coilv2.BlockRequest{}
			k8sClient.Get(ctx, client.ObjectKey{Name: "br-2"}, br)
			return br.Status.AddressBlockName
		}).ShouldNot(BeEmpty())

		By("checking that it does not allocate when there are no free blocks")
		br = &coilv2.BlockRequest{}
		br.Name = "br-3"
		br.Spec.NodeName = "node3"
		br.Spec.PoolName = "default"
		err = k8sClient.Create(ctx, br)
		Expect(err).To(Succeed())

		time.Sleep(10 * time.Millisecond)
		Expect(poolMgr.GetAllocated()).To(BeNumerically("==", 2))
	})
})
