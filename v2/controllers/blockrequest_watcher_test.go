package controllers

import (
	"context"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var _ = Describe("BlockRequest watcher", func() {
	ctx := context.Background()
	var cancel context.CancelFunc
	var nodeIPAM *mockNodeIPAM

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.TODO())
		nodeIPAM = &mockNodeIPAM{}
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:         scheme,
			LeaderElection: false,
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		brw := &BlockRequestWatcher{
			Client:   mgr.GetClient(),
			NodeIPAM: nodeIPAM,
			NodeName: "node2",
		}
		err = brw.SetupWithManager(mgr)
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
		time.Sleep(10 * time.Millisecond)
	})

	It("should notify requests", func() {
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

	It("should ignore requests of other nodes", func() {
		br := &coilv2.BlockRequest{}
		br.Name = "br-2"
		br.Spec.NodeName = "node1"
		br.Spec.PoolName = "default"
		err := k8sClient.Create(ctx, br)
		Expect(err).To(Succeed())
		br.Status.Conditions = []coilv2.BlockRequestCondition{
			{
				Type:               coilv2.BlockRequestComplete,
				Status:             corev1.ConditionTrue,
				LastProbeTime:      metav1.Now(),
				LastTransitionTime: metav1.Now(),
			},
		}
		br.Status.AddressBlockName = "default-1"
		err = k8sClient.Status().Update(ctx, br)
		Expect(err).To(Succeed())

		Consistently(func() int {
			return nodeIPAM.GetNotified()
		}).Should(Equal(0))
	})
})
