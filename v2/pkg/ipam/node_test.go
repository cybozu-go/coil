package ipam

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	. "github.com/cybozu-go/coil/v2/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func testController(ctx context.Context, npMap map[string]NodeIPAM) {
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()

	blocksMap := map[string][]*coilv2.AddressBlock{
		"default": {
			&coilv2.AddressBlock{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default-0",
					Labels: map[string]string{
						constants.LabelPool: "default",
					},
					Finalizers: []string{constants.FinCoil},
				},
				Index: 0,
				IPv4:  strPtr("10.2.0.0/31"),
				IPv6:  strPtr("fd02::0200/127"),
			},
			&coilv2.AddressBlock{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default-1",
					Labels: map[string]string{
						constants.LabelPool: "default",
					},
					Finalizers: []string{constants.FinCoil},
				},
				Index: 1,
				IPv4:  strPtr("10.2.0.2/31"),
				IPv6:  strPtr("fd02::0202/127"),
			},
		},
		"v4": {
			&coilv2.AddressBlock{
				ObjectMeta: metav1.ObjectMeta{
					Name: "v4-0",
					Labels: map[string]string{
						constants.LabelPool: "v4",
					},
					Finalizers: []string{constants.FinCoil},
				},
				Index: 0,
				IPv4:  strPtr("10.4.0.0/32"),
			},
		},
	}

	process := func(req *coilv2.BlockRequest) error {
		if len(req.Status.Conditions) > 0 {
			return nil
		}

		defer func() {
			np := npMap[req.Spec.NodeName]
			if np == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
			np.Notify(req)
		}()

		for _, block := range blocksMap[req.Spec.PoolName] {
			b := &coilv2.AddressBlock{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: block.Name}, b)
			if apierrors.IsNotFound(err) {
				block = block.DeepCopy()
				block.Labels[constants.LabelNode] = req.Spec.NodeName
				if err := k8sClient.Create(ctx, block); err != nil {
					return err
				}

				req.Status.Conditions = append(req.Status.Conditions, coilv2.BlockRequestCondition{
					Type:               coilv2.BlockRequestComplete,
					Status:             corev1.ConditionTrue,
					LastProbeTime:      metav1.Now(),
					LastTransitionTime: metav1.Now(),
				})
				req.Status.AddressBlockName = block.Name
				return k8sClient.Status().Update(ctx, req)
			}
			if err != nil {
				return err
			}
		}

		req.Status.Conditions = append(req.Status.Conditions,
			coilv2.BlockRequestCondition{
				Type:               coilv2.BlockRequestComplete,
				Status:             corev1.ConditionTrue,
				LastProbeTime:      metav1.Now(),
				LastTransitionTime: metav1.Now(),
			},
			coilv2.BlockRequestCondition{
				Type:               coilv2.BlockRequestFailed,
				Status:             corev1.ConditionTrue,
				Reason:             "no free blocks",
				LastProbeTime:      metav1.Now(),
				LastTransitionTime: metav1.Now(),
			},
		)
		ctrl.Log.Info("no block", "node", req.Spec.NodeName, "pool", req.Spec.PoolName)
		return k8sClient.Status().Update(ctx, req)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			reqs := &coilv2.BlockRequestList{}
			err := k8sClient.List(ctx, reqs)
			if err != nil {
				log.Log.Error(err, "failed to list requests")
				continue
			}
			for _, req := range reqs.Items {
				req := req
				if err := process(&req); err != nil {
					log.Log.Error(err, "failed to process request", "name", req.Name)
				}
			}
		}
	}
}

type mockExporter struct {
	subnets map[string]struct{}
}

func (m *mockExporter) Sync(subnets []*net.IPNet) error {
	m.subnets = make(map[string]struct{})
	for _, n := range subnets {
		m.subnets[n.String()] = struct{}{}
	}
	return nil
}

func (m *mockExporter) Equal(subnets []string) bool {
	t := make(map[string]struct{})
	for _, n := range subnets {
		t[n] = struct{}{}
	}
	return reflect.DeepEqual(m.subnets, t)
}

var _ = Describe("NodeIPAM", func() {
	ctx := context.Background()

	AfterEach(func() {
		cleanBlocks()
	})

	It("should timeout if there is no working controller", func() {
		nodeIPAM := NewNodeIPAM("node1", ctrl.Log.WithName("NodeIPAM"), mgr, nil)

		ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		_, _, err := nodeIPAM.Allocate(ctx, "default", "c0", "eth0")
		Expect(err).To(MatchError(context.DeadlineExceeded))
	})

	It("should acquire block and allocate IP addresses", func() {
		e1 := &mockExporter{}
		e2 := &mockExporter{}
		nodeIPAM := NewNodeIPAM("node1", ctrl.Log.WithName("NodeIPAM1"), mgr, e1)
		nodeIPAM2 := NewNodeIPAM("node2", ctrl.Log.WithName("NodeIPAM2"), mgr, e2)

		// run the dummy controller
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go testController(ctx, map[string]NodeIPAM{
			"node1": nodeIPAM,
			"node2": nodeIPAM2,
		})

		ipv4, ipv6, err := nodeIPAM.Allocate(ctx, "default", "c0", "eth0")
		Expect(err).ToNot(HaveOccurred())
		Expect(ipv4).To(EqualIP(net.ParseIP("10.2.0.0")))
		Expect(ipv6).To(EqualIP(net.ParseIP("fd02::0200")))
		Expect(e1.Equal([]string{"10.2.0.0/31", "fd02::200/127"})).To(BeTrue())

		for i := 0; i < 3; i++ {
			_, _, err := nodeIPAM.Allocate(ctx, "default", fmt.Sprintf("c%d", i+1), "eth0")
			Expect(err).ToNot(HaveOccurred())
		}
		Expect(e1.Equal([]string{
			"10.2.0.0/31",
			"10.2.0.2/31",
			"fd02::200/127",
			"fd02::202/127",
		})).To(BeTrue())

		_, _, err = nodeIPAM.Allocate(ctx, "default", "cxx", "eth0")
		Expect(err).To(HaveOccurred())

		err = nodeIPAM.Free(ctx, "c2", "eth0")
		Expect(err).NotTo(HaveOccurred())

		ipv4, ipv6, err = nodeIPAM.Allocate(ctx, "default", "c100", "eth0")
		Expect(err).ToNot(HaveOccurred())
		Expect(ipv4).To(EqualIP(net.ParseIP("10.2.0.2")))
		Expect(ipv6).To(EqualIP(net.ParseIP("fd02::0202")))

		_, _, err = nodeIPAM2.Allocate(ctx, "default", "d0", "eth0")
		Expect(err).To(HaveOccurred())

		ipv4, ipv6, err = nodeIPAM2.Allocate(ctx, "v4", "d1", "eth0")
		Expect(err).ToNot(HaveOccurred())
		Expect(ipv4).To(EqualIP(net.ParseIP("10.4.0.0")))
		Expect(ipv6).To(BeNil())
		Expect(e2.Equal([]string{"10.4.0.0/32"})).To(BeTrue())

		err = nodeIPAM2.Free(ctx, "d1", "eth0")
		Expect(err).NotTo(HaveOccurred())

		ipv4, ipv6, err = nodeIPAM.Allocate(ctx, "v4", "c101", "eth0")
		Expect(err).ToNot(HaveOccurred())
		Expect(ipv4).To(EqualIP(net.ParseIP("10.4.0.0")))
		Expect(ipv6).To(BeNil())
	}, 5)

	It("can restore state and return unused blocks", func() {
		nodeIPAM := NewNodeIPAM("node1", ctrl.Log.WithName("NodeIPAM3"), mgr, nil)

		// run the dummy controller
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go testController(ctx, map[string]NodeIPAM{
			"node1": nodeIPAM,
		})

		_, _, err := nodeIPAM.Allocate(ctx, "default", "c0", "eth0")
		Expect(err).ToNot(HaveOccurred())
		_, _, err = nodeIPAM.Allocate(ctx, "default", "c0", "eth1")
		Expect(err).ToNot(HaveOccurred())
		ipv4, ipv6, err := nodeIPAM.Allocate(ctx, "default", "c0", "eth2")
		Expect(err).ToNot(HaveOccurred())
		Expect(ipv4).To(EqualIP(net.ParseIP("10.2.0.2")))
		Expect(ipv6).To(EqualIP(net.ParseIP("fd02::0202")))

		// confirm that 2 blocks are assigned
		blocks := &coilv2.AddressBlockList{}
		err = k8sClient.List(ctx, blocks)
		Expect(err).ToNot(HaveOccurred())
		Expect(blocks.Items).To(HaveLen(2))

		// recreate node IPAM
		e1 := &mockExporter{}
		nodeIPAM = NewNodeIPAM("node1", ctrl.Log.WithName("NodeIPAM-recreated"), mgr, e1)
		err = nodeIPAM.Register(ctx, "default", "c0", "eth2", ipv4, ipv6)
		Expect(err).ToNot(HaveOccurred())

		err = nodeIPAM.GC(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(e1.Equal([]string{"10.2.0.2/31", "fd02::202/127"})).To(BeTrue())

		// confirm that 1 unused block is returned
		blocks = &coilv2.AddressBlockList{}
		err = k8sClient.List(ctx, blocks)
		Expect(err).ToNot(HaveOccurred())
		Expect(blocks.Items).To(HaveLen(1))

		ipv4, ipv6, err = nodeIPAM.Allocate(ctx, "default", "c0", "eth3")
		Expect(err).ToNot(HaveOccurred())
		Expect(ipv4).To(EqualIP(net.ParseIP("10.2.0.3")))
		Expect(ipv6).To(EqualIP(net.ParseIP("fd02::0203")))
	}, 5)

	It("should ignore reserved blocks", func() {
		By("creating a reserved block")
		block := &coilv2.AddressBlock{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default-2",
				Labels: map[string]string{
					constants.LabelPool:     "default",
					constants.LabelNode:     "node1",
					constants.LabelReserved: "true",
				},
				Finalizers: []string{constants.FinCoil},
			},
			Index: 2,
			IPv4:  strPtr("10.2.0.4/31"),
			IPv6:  strPtr("fd02::0204/127"),
		}
		err := k8sClient.Create(ctx, block)
		Expect(err).ShouldNot(HaveOccurred())

		nodeIPAM := NewNodeIPAM("node1", ctrl.Log.WithName("NodeIPAM3"), mgr, nil)

		// run the dummy controller
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go testController(ctx, map[string]NodeIPAM{
			"node1": nodeIPAM,
		})

		_, _, err = nodeIPAM.Allocate(ctx, "default", "c0", "eth0")
		Expect(err).ToNot(HaveOccurred())

		// confirm that another block was assigned
		blocks := &coilv2.AddressBlockList{}
		err = k8sClient.List(ctx, blocks)
		Expect(err).ToNot(HaveOccurred())
		Expect(blocks.Items).To(HaveLen(2))
	}, 5)
})
