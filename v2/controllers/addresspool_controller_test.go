package controllers

import (
	"context"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AddressPool reconciler", func() {
	ctx := context.Background()
	var stopCh chan struct{}
	var poolMgr *mockPoolManager

	BeforeEach(func() {
		stopCh = make(chan struct{})
		poolMgr = &mockPoolManager{}
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ToNot(HaveOccurred())

		apr := AddressPoolReconciler{
			Client:  mgr.GetClient(),
			Log:     ctrl.Log.WithName("AddressPool reconciler"),
			Manager: poolMgr,
			Scheme:  mgr.GetScheme(),
		}
		err = apr.SetupWithManager(mgr)
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
		ap := &coilv2.AddressPool{}
		ap.Name = "default"
		ap.Spec.BlockSizeBits = 1
		ap.Spec.Subnets = []coilv2.SubnetSet{
			{IPv4: strPtr("10.2.0.0/29"), IPv6: strPtr("fd02::0200/125")},
			{IPv4: strPtr("10.3.0.0/30"), IPv6: strPtr("fd02::0300/126")},
		}
		k8sClient.Create(ctx, ap)
		time.Sleep(10 * time.Millisecond)
	})

	It("works as expected", func() {
		By("checking the synchronization status after startup")
		Eventually(func() map[string]int {
			return poolMgr.GetSynced()
		}).Should(Equal(map[string]int{
			"default": 1,
			"v4":      1,
		}))
		Expect(poolMgr.GetDropped()).To(BeEmpty())

		By("checking the synchronization status upon AddressBlock creation")
		ap := &coilv2.AddressPool{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: "default"}, ap)
		Expect(err).To(Succeed())

		b := &coilv2.AddressBlock{}
		b.Name = "default-0"
		ctrl.SetControllerReference(ap, b, scheme)
		err = k8sClient.Create(ctx, b)
		Expect(err).To(Succeed())
		time.Sleep(10 * time.Millisecond)

		Expect(poolMgr.GetSynced()).To(Equal(map[string]int{
			"default": 1,
			"v4":      1,
		}))

		By("checking the synchronization status after AddressBlock deletion")
		err = k8sClient.Delete(ctx, b)
		Expect(err).To(Succeed())
		Eventually(func() map[string]int {
			return poolMgr.GetSynced()
		}).Should(Equal(map[string]int{
			"default": 2,
			"v4":      1,
		}))

		By("checking the drop status after pool deletion")
		err = k8sClient.Delete(ctx, ap)
		Expect(err).To(Succeed())
		Eventually(func() map[string]int {
			return poolMgr.GetDropped()
		}).Should(Equal(map[string]int{
			"default": 1,
		}))
	})
})
