package runners

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	current "github.com/containernetworking/cni/pkg/types/100"
	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/cnirpc"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/expfmt"
	"github.com/vishvananda/netlink"
	uberzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type mockNodeIPAM struct {
	nAllocate int
	nFree     int
	errFree   bool
}

func (n *mockNodeIPAM) Register(ctx context.Context, poolName, containerID, iface string, ipv4, ipv6 net.IP) error {
	panic("not implemented")
}
func (n *mockNodeIPAM) GC(ctx context.Context) error {
	panic("not implemented")
}
func (n *mockNodeIPAM) Notify(*coilv2.BlockRequest) {
	panic("not implemented")
}
func (n *mockNodeIPAM) NodeInternalIP(ctx context.Context) (net.IP, net.IP, error) {
	panic("not implemented")
}

func (n *mockNodeIPAM) Allocate(ctx context.Context, poolName, containerID, iface string) (ipv4, ipv6 net.IP, err error) {
	n.nAllocate++
	if poolName == "default" {
		switch containerID {
		case "pod1":
			return net.ParseIP("10.1.2.3"), net.ParseIP("fd02::1"), nil
		case "nat-client1":
			return net.ParseIP("10.1.2.4"), net.ParseIP("fd02::2"), nil
		case "nat-client2":
			return net.ParseIP("10.1.2.5"), net.ParseIP("fd02::3"), nil
		}
	}
	if poolName == "global" && containerID == "dns1" {
		return net.ParseIP("8.8.8.8"), nil, nil
	}
	return nil, nil, errors.New("some error")
}

func (n *mockNodeIPAM) Free(ctx context.Context, containerID, iface string) error {
	n.nFree++
	if n.errFree {
		return errors.New("free failure")
	}
	return nil
}

type mockPodNetwork struct {
	nSetup   int
	nCheck   int
	nDestroy int

	errSetup   bool
	errDestroy bool
}

func (p *mockPodNetwork) Init() error {
	panic("not implemented")
}
func (p *mockPodNetwork) List() ([]*nodenet.PodNetConf, error) {
	panic("not implemented")
}

func (p *mockPodNetwork) SetupIPAM(nsPath, podName, podNS string, conf *nodenet.PodNetConf) (*current.Result, error) {
	p.nSetup++
	if p.errSetup {
		return nil, errors.New("setup failure")
	}
	var ips []*current.IPConfig
	if conf.IPv4 != nil {
		ips = append(ips, &current.IPConfig{
			Address: *netlink.NewIPNet(conf.IPv4),
		})
	}
	if conf.IPv6 != nil {
		ips = append(ips, &current.IPConfig{
			Address: *netlink.NewIPNet(conf.IPv6),
		})
	}
	return &current.Result{IPs: ips}, nil
}

func (p *mockPodNetwork) SetupEgress(nsPath string, conf *nodenet.PodNetConf, hook nodenet.SetupHook) error {
	return nil
}

func (p *mockPodNetwork) Update(podIPv4, podIPv6 net.IP, hook nodenet.SetupHook, pod *corev1.Pod) error {
	panic("not implemented")
}

func (p *mockPodNetwork) Check(containerId, iface string) error {
	p.nCheck++
	if containerId == "pod1" || containerId == "dns1" {
		return nil
	}
	return errors.New("check failure")
}

func (p *mockPodNetwork) Destroy(containerId, iface string) error {
	p.nDestroy++
	if p.errDestroy {
		return errors.New("destroy failure")
	}
	return nil
}

type mockNATSetup struct {
	gwnets []GWNets
}

func (ns *mockNATSetup) Hook(gwnets []GWNets, _ *uberzap.Logger) func(ipv4, ipv6 net.IP) error {
	ns.gwnets = gwnets
	return nil
}

var _ = Describe("Coild server", func() {
	tmpFile, err := os.CreateTemp("", "")
	if err != nil {
		panic(err)
	}
	coildSocket := tmpFile.Name()
	tmpFile.Close()
	os.Remove(coildSocket)

	ctx := context.Background()
	var cancel context.CancelFunc
	var nodeIPAM *mockNodeIPAM
	var podNet *mockPodNetwork
	var natsetup *mockNATSetup
	var logbuf *bytes.Buffer
	var conn *grpc.ClientConn
	var cniClient cnirpc.CNIClient
	metricPort := 13449

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.TODO())
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:         scheme,
			LeaderElection: false,
			Metrics: metricsserver.Options{
				BindAddress: fmt.Sprintf("localhost:%d", metricPort),
			},
		})
		metricPort--
		Expect(err).ToNot(HaveOccurred())

		l, err := net.Listen("unix", coildSocket)
		if err != nil {
			Expect(err).ToNot(HaveOccurred())
		}
		nodeIPAM = &mockNodeIPAM{}
		podNet = &mockPodNetwork{}
		natsetup = &mockNATSetup{}
		logbuf = &bytes.Buffer{}
		logger := zap.NewRaw(zap.WriteTo(logbuf), zap.StacktraceLevel(zapcore.DPanicLevel))
		serv := NewCoildServer(l, mgr, nodeIPAM, podNet, natsetup, logger)
		err = mgr.Add(serv)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			err := mgr.Start(ctx)
			if err != nil {
				panic(err)
			}
		}()
		time.Sleep(100 * time.Millisecond)

		dialer := &net.Dialer{}
		dialFunc := func(ctx context.Context, a string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", a)
		}
		resolver.SetDefaultScheme("passthrough")
		conn, err = grpc.NewClient(coildSocket, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(dialFunc))
		Expect(err).ToNot(HaveOccurred())
		cniClient = cnirpc.NewCNIClient(conn)
	})

	AfterEach(func() {
		if conn != nil {
			conn.Close()
			conn = nil
		}
		cancel()
		time.Sleep(10 * time.Millisecond)
		os.Remove(coildSocket)
	})

	It("should work as expected", func() {
		By("calling Add without creating the pod")
		_, err := cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "foo", "K8S_POD_NAMESPACE": "ns1"},
			ContainerId: "pod1",
			Ifname:      "eth0",
			Netns:       "/run/netns/foo",
		})
		Expect(err).To(HaveOccurred())

		pod := &corev1.Pod{}
		pod.Namespace = "ns1"
		pod.Name = "foo"
		pod.Spec.Containers = []corev1.Container{
			{Name: "foo", Image: "nginx"},
		}
		err = k8sClient.Create(ctx, pod)
		Expect(err).NotTo(HaveOccurred())

		By("calling Add w/o K8S_POD_NAMESPACE")
		_, err = cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "foo"}, // no namespace
			ContainerId: "pod1",
			Ifname:      "eth0",
			Netns:       "/run/netns/foo",
		})
		Expect(err).To(HaveOccurred())

		By("calling Add w/o K8S_POD_NAME")
		_, err = cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAMESPACE": "ns1"}, // no name
			ContainerId: "pod1",
			Ifname:      "eth0",
			Netns:       "/run/netns/foo",
		})
		Expect(err).To(HaveOccurred())

		By("calling Add with enough parameters")
		logbuf.Reset()
		data, err := cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "foo", "K8S_POD_NAMESPACE": "ns1"},
			ContainerId: "pod1",
			Ifname:      "eth0",
			Netns:       "/run/netns/foo",
		})
		Expect(err).NotTo(HaveOccurred())

		By("checking the result")
		result := &current.Result{}
		err = json.Unmarshal(data.Result, result)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IPs).To(HaveLen(2))

		By("checking custom tags in gRPC log")
		// Expecting JSON output like:
		// {
		//   "grpc.code": "OK",
		//   "grpc.method": "Add",
		//   "grpc.request.pod.name": "foo",
		//   "grpc.request.pod.namespace": "ns1",
		//   "grpc.service": "pkg.cnirpc.CNI",
		//   "grpc.start_time": "2020-08-30T10:18:49Z",
		//   "grpc.time_ms": 102.34100341796875,
		//   "level": "info",
		//   "msg": "finished unary call with code OK",
		//   "peer.address": "@",
		//   "span.kind": "server",
		//   "system": "grpc",
		//   "ts": 1598782729.4605289
		// }
		logFields := struct {
			Method      string `json:"grpc.method"`
			Netns       string `json:"grpc.request.netns"`
			ContainerId string `json:"grpc.request.container_id"`
			Ifname      string `json:"grpc.request.ifname"`
			PodName     string `json:"grpc.request.pod.name"`
			PodNS       string `json:"grpc.request.pod.namespace"`
		}{}
		err = json.Unmarshal(logbuf.Bytes(), &logFields)
		Expect(err).ToNot(HaveOccurred())
		Expect(logFields.Method).To(Equal("Add"))
		Expect(logFields.Netns).To(Equal("/run/netns/foo"))
		Expect(logFields.ContainerId).To(Equal("pod1"))
		Expect(logFields.Ifname).To(Equal("eth0"))
		Expect(logFields.PodName).To(Equal("foo"))
		Expect(logFields.PodNS).To(Equal("ns1"))

		By("checking metrics for gRPC")
		resp, err := http.Get("http://localhost:13449/metrics")
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		mfs, err := (&expfmt.TextParser{}).TextToMetricFamilies(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(mfs).To(HaveKey("grpc_server_handled_total"))
		mf := mfs["grpc_server_handled_total"]
		metric := findMetric(mf, map[string]string{
			"grpc_method": "Add",
			"grpc_code":   "OK",
		})
		Expect(metric).NotTo(BeNil())
		Expect(metric.GetCounter().GetValue()).To(BeNumerically("==", 1))

		By("creating a pod in ns2")
		pod = &corev1.Pod{}
		pod.Namespace = "ns2"
		pod.Name = "bar"
		pod.Spec.Containers = []corev1.Container{
			{Name: "nginx", Image: "nginx"},
		}
		err = k8sClient.Create(ctx, pod)
		Expect(err).NotTo(HaveOccurred())

		By("calling Add expecting a temporary failure")
		podNet.errSetup = true
		_, err = cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "bar", "K8S_POD_NAMESPACE": "ns2"},
			ContainerId: "dns1",
			Ifname:      "eth0",
			Netns:       "/run/netns/bar",
		})
		Expect(err).To(HaveOccurred())

		By("calling Add for ns2/bar")
		podNet.errSetup = false
		data, err = cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "bar", "K8S_POD_NAMESPACE": "ns2"},
			ContainerId: "dns1",
			Ifname:      "eth0",
			Netns:       "/run/netns/bar",
		})
		Expect(err).NotTo(HaveOccurred())

		By("checking the result")
		result = &current.Result{}
		err = json.Unmarshal(data.Result, result)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IPs).To(HaveLen(1))
		Expect(result.IPs[0].Address.IP.To4()).To(Equal(net.ParseIP("8.8.8.8").To4()))

		By("creating another pod in ns1")
		pod = &corev1.Pod{}
		pod.Namespace = "ns1"
		pod.Name = "zot"
		pod.Spec.Containers = []corev1.Container{
			{Name: "nginx", Image: "nginx"},
		}
		err = k8sClient.Create(ctx, pod)
		Expect(err).NotTo(HaveOccurred())

		By("calling Add to fail for Allocation failure")
		_, err = cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "zot", "K8S_POD_NAMESPACE": "ns1"},
			ContainerId: "hoge",
			Ifname:      "eth0",
			Netns:       "/run/netns/zot",
		})
		Expect(err).To(HaveOccurred())

		By("calling Check")
		_, err = cniClient.Check(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "bar", "K8S_POD_NAMESPACE": "ns2"},
			ContainerId: "dns1",
			Ifname:      "eth0",
			Netns:       "/run/netns/bar",
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = cniClient.Check(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "zot", "K8S_POD_NAMESPACE": "ns1"},
			ContainerId: "hoge",
			Ifname:      "eth0",
			Netns:       "/run/netns/zot",
		})
		Expect(err).To(HaveOccurred())

		By("calling Del")
		_, err = cniClient.Del(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "bar", "K8S_POD_NAMESPACE": "ns2"},
			ContainerId: "dns1",
			Ifname:      "eth0",
			Netns:       "/run/netns/bar",
		})
		Expect(err).NotTo(HaveOccurred())

		nodeIPAM.errFree = true
		_, err = cniClient.Del(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "bar", "K8S_POD_NAMESPACE": "ns2"},
			ContainerId: "dns1",
			Ifname:      "eth0",
			Netns:       "/run/netns/bar",
		})
		Expect(err).To(HaveOccurred())

		nodeIPAM.errFree = false
		podNet.errDestroy = true
		_, err = cniClient.Del(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "bar", "K8S_POD_NAMESPACE": "ns2"},
			ContainerId: "dns1",
			Ifname:      "eth0",
			Netns:       "/run/netns/bar",
		})
		Expect(err).To(HaveOccurred())
	})

	It("should setup Foo-over-UDP NAT", func() {
		By("creating pod declaring itself as a NAT client")
		pod := &corev1.Pod{}
		pod.Namespace = "ns1"
		pod.Name = "nat-client1"
		pod.Spec.Containers = []corev1.Container{
			{Name: "foo", Image: "nginx"},
		}
		pod.Annotations = map[string]string{
			"egress.coil.cybozu.com/ns2": "egress",
		}
		err = k8sClient.Create(ctx, pod)
		Expect(err).NotTo(HaveOccurred())

		By("calling Add without Egress/Service")
		_, err := cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "nat-client1", "K8S_POD_NAMESPACE": "ns1"},
			ContainerId: "nat-client1",
			Ifname:      "eth0",
			Netns:       "/run/netns/nat-client1",
		})
		Expect(err).To(HaveOccurred())

		eg := &coilv2.Egress{}
		eg.Namespace = "ns2"
		eg.Name = "egress"
		eg.Spec.Destinations = []string{"192.168.0.0/16", "fd20::/112"}
		eg.Spec.Replicas = 1
		err = k8sClient.Create(ctx, eg)
		Expect(err).NotTo(HaveOccurred())

		By("calling Add without Service")
		_, err = cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "nat-client1", "K8S_POD_NAMESPACE": "ns1"},
			ContainerId: "nat-client1",
			Ifname:      "eth0",
			Netns:       "/run/netns/nat-client1",
		})
		Expect(err).To(HaveOccurred())

		svc := &corev1.Service{}
		svc.Namespace = "ns2"
		svc.Name = "egress"
		// currently, ClusterIP must be picked from 10.0.0.0/24
		// see https://github.com/kubernetes/kubernetes/pull/51249
		svc.Spec.ClusterIP = "10.0.0.5"
		svc.Spec.Ports = []corev1.ServicePort{{Port: 8080}}
		err = k8sClient.Create(ctx, svc)
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(100 * time.Millisecond)

		By("calling Add with IPv4 Service")
		_, err = cniClient.Add(ctx, &cnirpc.CNIArgs{
			Args:        map[string]string{"K8S_POD_NAME": "nat-client1", "K8S_POD_NAMESPACE": "ns1"},
			ContainerId: "nat-client1",
			Ifname:      "eth0",
			Netns:       "/run/netns/nat-client1",
		})
		Expect(err).NotTo(HaveOccurred())

		By("checking NAT configurations")
		Expect(natsetup.gwnets).To(HaveLen(1))
		gwnets := natsetup.gwnets[0]
		Expect(gwnets.Gateway.Equal(net.ParseIP("10.0.0.5"))).To(BeTrue())
		Expect(gwnets.Networks).To(HaveLen(1))
		subnet := gwnets.Networks[0]
		Expect(subnet.IP.Equal(net.ParseIP("192.168.0.0"))).To(BeTrue())
	})
})
