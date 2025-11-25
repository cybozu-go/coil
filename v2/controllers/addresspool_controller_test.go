package controllers

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
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
				SkipNameValidation: ptr.To(true),
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
			{IPv4: strPtr("10.2.0.0/29"), IPv6: strPtr("fd02::0200/125")},
			{IPv4: strPtr("10.3.0.0/30"), IPv6: strPtr("fd02::0300/126")},
		}
		k8sClient.Create(context.Background(), ap)
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

		By("marking the pool as not-used")
		poolMgr.SetUsed(false)

		// Update the pool to trigger reconciliation.
		// In the real environment, reconciliation will be triggered by the deletion of dependent AddressBlocks.
		err = k8sClient.Get(ctx, client.ObjectKey{Name: "default"}, ap)
		Expect(err).To(Succeed())
		if ap.Annotations == nil {
			ap.Annotations = make(map[string]string)
		}
		ap.Annotations["foo"] = "bar"
		err = k8sClient.Update(ctx, ap)
		Expect(err).To(Succeed())

		Eventually(func() error {
			ap := &coilv2.AddressPool{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "default"}, ap)
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}

			return errors.New("pool still exists")
		}).Should(Succeed())
	})
})
