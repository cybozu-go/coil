package controllers

import (
	"context"
	"net"
	"reflect"
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
)

var _ = Describe("Egress watcher", func() {
	ctx := context.Background()
	podNetwork := &mockPodNetwork{ips: make(map[string]bool)}
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
		})
		makePod("pod2", []string{"10.1.1.3", "fd01::3"}, map[string]string{
			"default": "egress2",
		})
		makePod("pod3", []string{"10.1.1.4", "fd01::4"}, map[string]string{
			"internet": "egress1",
		})

		ctx, cancel = context.WithCancel(context.TODO())
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
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
		Eventually(func() bool {
			return reflect.DeepEqual(podNetwork.updatedPodIPs(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
			})
		}).Should(BeTrue())

		Consistently(func() bool {
			return reflect.DeepEqual(podNetwork.updatedPodIPs(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
			})
		}, 5*time.Second, 1*time.Second).Should(BeTrue())
	})

})

type mockPodNetwork struct {
	nUpdate int
	ips     map[string]bool

	mu sync.Mutex
}

func (p *mockPodNetwork) Init() error {
	panic("not implemented")
}
func (p *mockPodNetwork) List() ([]*nodenet.PodNetConf, error) {
	panic("not implemented")
}

func (p *mockPodNetwork) Setup(nsPath, podName, podNS string, conf *nodenet.PodNetConf, hook nodenet.SetupHook) (*current.Result, error) {
	panic("not implemented")
}

func (p *mockPodNetwork) Update(podIPv4, podIPv6 net.IP, hook nodenet.SetupHook) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.nUpdate++
	p.ips[podIPv4.String()] = true
	p.ips[podIPv6.String()] = true
	return nil
}

func (p *mockPodNetwork) updatedPodIPs() map[string]bool {
	m := make(map[string]bool)

	p.mu.Lock()
	defer p.mu.Unlock()

	for k := range p.ips {
		m[k] = true
	}
	return m
}

func (p *mockPodNetwork) Check(containerId, iface string) error {
	panic("not implemented")
}

func (p *mockPodNetwork) Destroy(containerId, iface string) error {
	panic("not implemented")
}
