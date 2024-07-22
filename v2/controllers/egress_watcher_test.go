package controllers

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	current "github.com/containernetworking/cni/pkg/types/100"
	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/indexing"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var _ = Describe("Egress watcher", func() {
	ctx := context.Background()
	podNetwork := &mockPodNetwork{ips: make(map[string]int)}
	var cancel context.CancelFunc

	BeforeEach(func() {
		eg := makeEgress("egress1")
		err := k8sClient.Create(ctx, eg)
		Expect(err).ShouldNot(HaveOccurred())

		svc := &corev1.Service{}
		svc.Namespace = "default"
		svc.Name = "egress1"
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Ports = []corev1.ServicePort{{
			Port:       5555,
			TargetPort: intstr.FromInt(5555),
			Protocol:   corev1.ProtocolUDP,
		}}
		err = k8sClient.Create(ctx, svc)
		Expect(err).ShouldNot(HaveOccurred())

		makePod("pod1", []string{"10.1.1.2", "fd01::2"}, map[string]string{
			"default": "egress1",
		}, corev1.PodRunning)
		makePod("pod2", []string{"10.1.1.3", "fd01::3"}, map[string]string{
			"default": "egress2",
		}, corev1.PodRunning)
		makePod("pod3", []string{"10.1.1.4", "fd01::4"}, map[string]string{
			"internet": "egress1",
		}, corev1.PodRunning)
		makePod("pod4", []string{"10.1.1.5", "fd01::5"}, map[string]string{
			"default": "egress1",
		}, corev1.PodSucceeded)

		ctx, cancel = context.WithCancel(context.TODO())
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:         scheme,
			LeaderElection: false,
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(indexing.SetupIndexForPodByNodeName(ctx, mgr)).ToNot(HaveOccurred())

		watcher := EgressWatcher{
			Client:     mgr.GetClient(),
			NodeName:   "coil-worker",
			PodNet:     podNetwork,
			EgressPort: 5555,
		}
		err = watcher.SetupWithManager(mgr)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			err := mgr.Start(ctx)
			if err != nil {
				panic(err)
			}
		}()
	})

	AfterEach(func() {
		cancel()
		err := k8sClient.DeleteAllOf(context.Background(), &corev1.Pod{}, client.InNamespace("default"))
		Expect(err).ShouldNot(HaveOccurred())
		err = k8sClient.DeleteAllOf(context.Background(), &coilv2.Egress{}, client.InNamespace("default"))
		Expect(err).ShouldNot(HaveOccurred())
		time.Sleep(10 * time.Millisecond)
	})

	It("should update a NAT setup for pod1", func() {
		eg := &coilv2.Egress{}
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "egress1"}, eg)
		Expect(err).ToNot(HaveOccurred())

		eg.Spec.FouSourcePortAuto = true
		err = k8sClient.Update(ctx, eg)
		Expect(err).ToNot(HaveOccurred())

		// pod1 is a client of default/egress1, but pod2 and pod3 are the clients of other egresses
		// pod4 is not the target of the reconciliation because pod4 is not running.
		Eventually(func() error {
			updated := podNetwork.getPodIPCount()
			updatedIPv4, ok := updated["10.1.1.2"]
			if !ok || updatedIPv4 != 1 {
				return fmt.Errorf("pod1's IPv4 address is not updated")
			}
			updatedIPv6, ok := updated["fd01::2"]
			if !ok || updatedIPv6 != 1 {
				return fmt.Errorf("pod1's IPv4 address is not updated")
			}
			return nil
		}).Should(Succeed())

		Consistently(func() error {
			updated := podNetwork.getPodIPCount()
			updatedIPv4, ok := updated["10.1.1.2"]
			if !ok || updatedIPv4 != 1 {
				return fmt.Errorf("pod1's IPv4 address is not updated")
			}
			updatedIPv6, ok := updated["fd01::2"]
			if !ok || updatedIPv6 != 1 {
				return fmt.Errorf("pod1's IPv4 address is not updated")
			}
			return nil
		}, 5*time.Second, 1*time.Second).Should(Succeed())

		makePod("pod5", []string{"10.1.1.3", "fd01::3"}, map[string]string{
			"default": "egress2",
		}, corev1.PodRunning)

		eg2 := makeEgress("egress2")
		err = k8sClient.Create(ctx, eg2)
		Expect(err).ShouldNot(HaveOccurred())

		svc2 := &corev1.Service{}
		svc2.Namespace = "default"
		svc2.Name = "egress2"
		svc2.Spec.Type = corev1.ServiceTypeClusterIP
		svc2.Spec.Ports = []corev1.ServicePort{{
			Port:       5555,
			TargetPort: intstr.FromInt(5555),
			Protocol:   corev1.ProtocolUDP,
		}}
		err = k8sClient.Create(ctx, svc2)
		Expect(err).ShouldNot(HaveOccurred())

		// Reconciliation should fail because of duplicate address
		Consistently(func() error {
			updated := podNetwork.getPodIPCount()
			_, ok := updated["10.1.1.3"]
			if ok {
				return fmt.Errorf("pod2 and pod5 should not be updated")
			}
			_, ok = updated["fd01::3"]
			if ok {
				return fmt.Errorf("pod2 and pod5 should not be updated")
			}
			return nil
		}, 5*time.Second, 1*time.Second).Should(Succeed())
	})
})

type mockPodNetwork struct {
	nUpdate int
	ips     map[string]int

	mu sync.Mutex
}

func (p *mockPodNetwork) Init() error {
	panic("not implemented")
}
func (p *mockPodNetwork) List() ([]*nodenet.PodNetConf, error) {
	panic("not implemented")
}

func (p *mockPodNetwork) SetupIPAM(nsPath, podName, podNS string, conf *nodenet.PodNetConf) (*current.Result, error) {
	panic("not implemented")
}

func (p *mockPodNetwork) SetupEgress(nsPath string, conf *nodenet.PodNetConf, hook nodenet.SetupHook) error {
	panic("not implemented")
}

func (p *mockPodNetwork) Update(podIPv4, podIPv6 net.IP, hook nodenet.SetupHook, pod *corev1.Pod) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.nUpdate++
	c4, ok := p.ips[podIPv4.String()]
	if !ok {
		p.ips[podIPv4.String()] = 1
	} else {
		c4 += 1
	}
	c6, ok := p.ips[podIPv6.String()]
	if !ok {
		p.ips[podIPv6.String()] = 1
	} else {
		c6 += 1
	}
	return nil
}

func (p *mockPodNetwork) getPodIPCount() map[string]int {
	m := make(map[string]int)

	p.mu.Lock()
	defer p.mu.Unlock()

	for k, v := range p.ips {
		m[k] = v
	}

	return m
}

func (p *mockPodNetwork) Check(containerId, iface string) error {
	panic("not implemented")
}

func (p *mockPodNetwork) Destroy(containerId, iface string) error {
	panic("not implemented")
}
