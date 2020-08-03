package controllers

import (
	"context"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BlockRequest reconciler", func() {
	ctx := context.Background()
	var stopCh chan struct{}
	var poolMgr *mockPoolManager

	BeforeEach(func() {
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

		stopCh = make(chan struct{})
		poolMgr = &mockPoolManager{}
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ToNot(HaveOccurred())

		brr := &BlockRequestReconciler{
			Client:  mgr.GetClient(),
			Log:     ctrl.Log.WithName("BlockRequest reconciler"),
			Manager: poolMgr,
			Scheme:  mgr.GetScheme(),
		}
		err = brr.SetupWithManager(mgr)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			err := mgr.Start(stopCh)
			if err != nil {
				panic(err)
			}
		}()
	})

	AfterEach(func() {
		close(stopCh)
		err := k8sClient.DeleteAllOf(ctx, &coilv2.BlockRequest{})
		Expect(err).To(Succeed())
		time.Sleep(10 * time.Millisecond)
	})

	It("works as expected", func() {
		By("checking that it reconciles unhandled request at the startup")
		Eventually(func() int {
			return poolMgr.GetAllocated()
		}).Should(Equal(1))

		By("checking that it can reconcile quickly upon a new request")
		br := &coilv2.BlockRequest{}
		br.Name = "br-2"
		br.Spec.NodeName = "node2"
		br.Spec.PoolName = "default"
		err := k8sClient.Create(ctx, br)
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
