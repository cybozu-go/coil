package controllers

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
)

var _ = Describe("AddressPool reconciler", func() {
	ctx := context.Background()
	var cancel context.CancelFunc
	var poolMgr *mockPoolManager

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.TODO())
		poolMgr = &mockPoolManager{}
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:         scheme,
			LeaderElection: false,
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
			Controller: config.Controller{
				SkipNameValidation: new(true),
			},
		})
		Expect(err).ToNot(HaveOccurred())

		apr := AddressPoolReconciler{
			Client:  mgr.GetClient(),
			Manager: poolMgr,
			Scheme:  mgr.GetScheme(),
		}
		err = apr.SetupWithManager(mgr)
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
		ap := &coilv2.AddressPool{}
		ap.Name = "default"
		ap.Spec.BlockSizeBits = 1
		ap.Spec.Subnets = []coilv2.SubnetSet{
			{IPv4: new("10.2.0.0/29"), IPv6: new("fd02::0200/125")},
			{IPv4: new("10.3.0.0/30"), IPv6: new("fd02::0300/126")},
		}
		_ = k8sClient.Create(context.Background(), ap)
		time.Sleep(10 * time.Millisecond)
	})

	It("should synchronize pools", func() {
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
		err = ctrl.SetControllerReference(ap, b, scheme)
		Expect(err).To(Succeed())

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

	It("should handle finalizers", func() {
		By("adding the finalizer on behalf of webhook")
		ap := &coilv2.AddressPool{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: "default"}, ap)
		Expect(err).To(Succeed())

		controllerutil.AddFinalizer(ap, constants.FinCoil)
		err = k8sClient.Update(ctx, ap)
		Expect(err).To(Succeed())

		By("trying to delete the pool while it is marked as used-by-AddressBlock")
		poolMgr.SetUsed(true)
		err = k8sClient.Delete(ctx, ap)
		Expect(err).To(Succeed())
		Consistently(func() error {
			ap := &coilv2.AddressPool{}
			return k8sClient.Get(ctx, client.ObjectKey{Name: "default"}, ap)
		}).Should(Succeed())

		By("marking the pool as not-used and waiting for the pool to be removed")
		poolMgr.SetUsed(false)

		// Touch the pool to trigger reconciliation, then wait for it to be garbage-collected.
		// In the real environment, reconciliation will be triggered by the deletion of dependent AddressBlocks.
		// The reconciler may race ahead between Get and Update; treat NotFound as success in either step.
		Eventually(func() error {
			ap := &coilv2.AddressPool{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "default"}, ap); err != nil {
				return client.IgnoreNotFound(err)
			}
			if ap.Annotations == nil {
				ap.Annotations = make(map[string]string)
			}
			ap.Annotations["foo"] = "bar"
			if err := k8sClient.Update(ctx, ap); err != nil {
				return client.IgnoreNotFound(err)
			}
			return errors.New("pool still exists")
		}).Should(Succeed())
	})
})
