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
)

var _ = Describe("BlockRequest watcher", func() {
	ctx := context.Background()
	var stopCh chan struct{}
	var nodeIPAM *mockNodeIPAM

	BeforeEach(func() {
		stopCh = make(chan struct{})
		nodeIPAM = &mockNodeIPAM{}
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ToNot(HaveOccurred())

		brw := &BlockRequestWatcher{
			Client:   mgr.GetClient(),
			Log:      ctrl.Log.WithName("BlockRequest watcher"),
			NodeIPAM: nodeIPAM,
			Scheme:   mgr.GetScheme(),
		}
		err = brw.SetupWithManager(mgr)
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
		By("ignoring a new request")
		br := &coilv2.BlockRequest{}
		br.Name = "br-2"
		br.Spec.NodeName = "node2"
		br.Spec.PoolName = "default"
		err := k8sClient.Create(ctx, br)
		Expect(err).To(Succeed())
		time.Sleep(10 * time.Millisecond)
		Expect(nodeIPAM.GetNotified()).To(Equal(0))

		By("updating the request status and check it notifies quickly")
		br.Status.Conditions = []coilv2.BlockRequestCondition{
			{
				Type:               coilv2.BlockRequestComplete,
				Status:             corev1.ConditionTrue,
				LastProbeTime:      metav1.Now(),
				LastTransitionTime: metav1.Now(),
			},
		}
		br.Status.AddressBlockName = "default-0"
		err = k8sClient.Status().Update(ctx, br)
		Expect(err).To(Succeed())

		Eventually(func() int {
			return nodeIPAM.GetNotified()
		}).Should(Equal(1))
	})
})
