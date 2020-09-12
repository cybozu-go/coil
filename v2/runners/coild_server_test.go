package runners

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/containernetworking/cni/pkg/types/current"
	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/cnirpc"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/expfmt"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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

func (n *mockNodeIPAM) Allocate(ctx context.Context, poolName, containerID, iface string) (ipv4, ipv6 net.IP, err error) {
	n.nAllocate++
	if poolName == "default" && containerID == "pod1" {
		return net.ParseIP("10.1.2.3"), net.ParseIP("fd02::1"), nil
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

func (p *mockPodNetwork) Setup(nsPath, podName, podNS string, conf *nodenet.PodNetConf) (*current.Result, error) {
	p.nSetup++
	if p.errSetup {
		return nil, errors.New("setup failure")
	}
	var ips []*current.IPConfig
	if conf.IPv4 != nil {
		ips = append(ips, &current.IPConfig{
			Version: "4",
			Address: *netlink.NewIPNet(conf.IPv4),
		})
	}
	if conf.IPv6 != nil {
		ips = append(ips, &current.IPConfig{
			Version: "6",
			Address: *netlink.NewIPNet(conf.IPv6),
		})
	}
	return &current.Result{IPs: ips}, nil
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

var _ = Describe("Coild server", func() {
	ctx := context.Background()
	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		panic(err)
	}
	coildSocket := tmpFile.Name()
	tmpFile.Close()
	os.Remove(coildSocket)

	var stopCh chan struct{}
	var nodeIPAM *mockNodeIPAM
	var podNet *mockPodNetwork
	var logbuf *bytes.Buffer
	var conn *grpc.ClientConn
	var cniClient cnirpc.CNIClient

	BeforeEach(func() {
		stopCh = make(chan struct{})
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "localhost:13449",
		})
		Expect(err).ToNot(HaveOccurred())

		l, err := net.Listen("unix", coildSocket)
		if err != nil {
			Expect(err).ToNot(HaveOccurred())
		}
		nodeIPAM = &mockNodeIPAM{}
		podNet = &mockPodNetwork{}
		logbuf = &bytes.Buffer{}
		logger := zap.NewRaw(zap.WriteTo(logbuf), zap.StacktraceLevel(zapcore.DPanicLevel))
		serv := NewCoildServer(l, mgr, nodeIPAM, podNet, logger)
		err = mgr.Add(serv)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			err := mgr.Start(stopCh)
			if err != nil {
				panic(err)
			}
		}()
		time.Sleep(10 * time.Millisecond)

		dialer := &net.Dialer{}
		dialFunc := func(ctx context.Context, a string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", a)
		}
		conn, err = grpc.Dial(coildSocket, grpc.WithInsecure(), grpc.WithContextDialer(dialFunc))
		Expect(err).ToNot(HaveOccurred())
		cniClient = cnirpc.NewCNIClient(conn)
	})

	AfterEach(func() {
		if conn != nil {
			conn.Close()
			conn = nil
		}
		if stopCh != nil {
			close(stopCh)
			stopCh = nil
		}
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
})
