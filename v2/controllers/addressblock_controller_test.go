package controllers

import (
	"context"
	"errors"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("AddressBlock reconciler", func() {
	ctx := context.Background()
	var cancel context.CancelFunc
	notifyCh := make(chan struct{}, 100)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.TODO())
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ToNot(HaveOccurred())

		apr := AddressBlockReconciler{Notify: notifyCh}
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
		time.Sleep(10 * time.Millisecond)
	})

	It("works as expected", func() {
		By("checking the notification upon AddressBlock creation")
		b := &coilv2.AddressBlock{}
		b.Name = "notify-0"
		err := k8sClient.Create(ctx, b)
		Expect(err).To(Succeed())

		Eventually(func() error {
			select {
			case <-notifyCh:
				return nil
			default:
				time.Sleep(1 * time.Millisecond)
				return errors.New("not yet notified")
			}
		}).Should(Succeed())
	})
})
