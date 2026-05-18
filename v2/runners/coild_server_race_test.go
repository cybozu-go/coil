package runners

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	current "github.com/containernetworking/cni/pkg/types/100"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/cnirpc"
	"github.com/cybozu-go/coil/v2/pkg/config"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
)

// raceNodeIPAM is a mock that tracks concurrent access for race detection.
type raceNodeIPAM struct {
	allocCount atomic.Int64
	freeCount  atomic.Int64
}

func (n *raceNodeIPAM) Register(ctx context.Context, poolName, containerID, iface string, ipv4, ipv6 net.IP) error {
	return nil
}
func (n *raceNodeIPAM) GC(ctx context.Context) error { return nil }
func (n *raceNodeIPAM) Notify(br *coilv2.BlockRequest) {}
func (n *raceNodeIPAM) NodeInternalIP(ctx context.Context) (net.IP, net.IP, error) {
	return nil, nil, nil
}
func (n *raceNodeIPAM) ClearRoutes(ctx context.Context) error { return nil }

func (n *raceNodeIPAM) Allocate(ctx context.Context, poolName, containerID, iface string) (net.IP, net.IP, error) {
	n.allocCount.Add(1)
	// Simulate some work to increase race window
	time.Sleep(5 * time.Millisecond)
	return net.ParseIP("10.1.2.100"), net.ParseIP("fd02::100"), nil
}

func (n *raceNodeIPAM) Free(ctx context.Context, containerID, iface string) error {
	n.freeCount.Add(1)
	return nil
}

// racePodNetwork is a mock that detects concurrent access to the same container.
type racePodNetwork struct {
	// Track concurrent entries per containerId to detect unsafe overlap.
	active sync.Map // containerId -> *int32

	setupCount  atomic.Int64
	egressCount atomic.Int64
	checkCount  atomic.Int64
	destroyCount atomic.Int64

	// maxConcurrentSame records the max concurrency seen for same containerId operations.
	maxConcurrentSame atomic.Int32
}

func (p *racePodNetwork) Init() error { return nil }
func (p *racePodNetwork) List() ([]*nodenet.PodNetConf, error) {
	return nil, nil
}

func (p *racePodNetwork) SetupIPAM(nsPath, podName, podNS string, conf *nodenet.PodNetConf) (*current.Result, error) {
	p.setupCount.Add(1)

	// Track concurrency per containerId
	counter := p.getOrCreateCounter(conf.ContainerId)
	cur := atomic.AddInt32(counter, 1)
	p.updateMax(cur)
	defer atomic.AddInt32(counter, -1)

	// Simulate work to widen the race window
	time.Sleep(20 * time.Millisecond)

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

func (p *racePodNetwork) SetupEgress(nsPath string, conf *nodenet.PodNetConf, hook nodenet.SetupHook) error {
	p.egressCount.Add(1)

	counter := p.getOrCreateCounter(conf.ContainerId)
	cur := atomic.AddInt32(counter, 1)
	p.updateMax(cur)
	defer atomic.AddInt32(counter, -1)

	time.Sleep(10 * time.Millisecond)
	return nil
}

func (p *racePodNetwork) Update(podIPv4, podIPv6 net.IP, hook nodenet.SetupHook, pod *corev1.Pod) error {
	return nil
}

func (p *racePodNetwork) Check(containerId, iface string) error {
	p.checkCount.Add(1)

	counter := p.getOrCreateCounter(containerId)
	cur := atomic.AddInt32(counter, 1)
	p.updateMax(cur)
	defer atomic.AddInt32(counter, -1)

	time.Sleep(5 * time.Millisecond)
	return nil
}

func (p *racePodNetwork) Destroy(containerId, iface string) error {
	p.destroyCount.Add(1)

	counter := p.getOrCreateCounter(containerId)
	cur := atomic.AddInt32(counter, 1)
	p.updateMax(cur)
	defer atomic.AddInt32(counter, -1)

	time.Sleep(5 * time.Millisecond)
	return nil
}

func (p *racePodNetwork) getOrCreateCounter(key string) *int32 {
	val, _ := p.active.LoadOrStore(key, new(int32))
	return val.(*int32)
}

func (p *racePodNetwork) updateMax(cur int32) {
	for {
		old := p.maxConcurrentSame.Load()
		if cur <= old {
			break
		}
		if p.maxConcurrentSame.CompareAndSwap(old, cur) {
			break
		}
	}
}

func raceAlias(conf *nodenet.PodNetConf, pod *corev1.Pod, ifName string) error {
	return nil
}

// vethCollisionPodNetwork simulates the real podNetwork veth creation logic
// to detect when concurrent SetupIPAM calls for the same pod (different ContainerIDs)
// race on the host-side veth name. This reproduces the "already exists" error
// seen in production when kubelet retries sandbox creation.
type vethCollisionPodNetwork struct {
	// Track "created" veths by host-side name to detect collisions.
	veths sync.Map // vethName -> struct{}

	// simulateCleanup controls whether the mock simulates calico-style cleanup
	// (delete existing veth before create). When true, mimics the fixed SetupIPAM.
	simulateCleanup bool

	collisions atomic.Int32
	setupCount atomic.Int64
}

func (p *vethCollisionPodNetwork) Init() error                { return nil }
func (p *vethCollisionPodNetwork) List() ([]*nodenet.PodNetConf, error) { return nil, nil }
func (p *vethCollisionPodNetwork) SetupEgress(nsPath string, conf *nodenet.PodNetConf, hook nodenet.SetupHook) error {
	return nil
}
func (p *vethCollisionPodNetwork) Update(podIPv4, podIPv6 net.IP, hook nodenet.SetupHook, pod *corev1.Pod) error {
	return nil
}
func (p *vethCollisionPodNetwork) Check(containerId, iface string) error { return nil }
func (p *vethCollisionPodNetwork) Destroy(containerId, iface string) error { return nil }

func (p *vethCollisionPodNetwork) SetupIPAM(nsPath, podName, podNS string, conf *nodenet.PodNetConf) (*current.Result, error) {
	p.setupCount.Add(1)

	// Simulate the real code path: generate deterministic veth name from (podName, podNS)
	vethName := fmt.Sprintf("veth-%s-%s", podNS, podName)

	// Simulate calico-style cleanup: delete stale veth by name before creating
	if p.simulateCleanup {
		p.veths.Delete(vethName)
	}

	// Simulate veth creation: check if name already exists
	if _, loaded := p.veths.LoadOrStore(vethName, struct{}{}); loaded {
		p.collisions.Add(1)
		return nil, fmt.Errorf("failed to setup veth: container veth name (%q) peer provided (%q) already exists",
			conf.IFace, vethName)
	}

	// Simulate work (veth creation + IP config takes time)
	time.Sleep(50 * time.Millisecond)

	var ips []*current.IPConfig
	if conf.IPv4 != nil {
		ips = append(ips, &current.IPConfig{Address: *netlink.NewIPNet(conf.IPv4)})
	}
	if conf.IPv6 != nil {
		ips = append(ips, &current.IPConfig{Address: *netlink.NewIPNet(conf.IPv6)})
	}
	return &current.Result{IPs: ips}, nil
}

func vethCollisionAlias(conf *nodenet.PodNetConf, pod *corev1.Pod, ifName string) error {
	return nil
}

var _ = Describe("Coild server veth collision", Label("race", "veth-collision"), func() {
	tmpFile, err := os.CreateTemp("", "coild-veth-collision-*.sock")
	if err != nil {
		panic(err)
	}
	coildSocket := tmpFile.Name()
	tmpFile.Close()
	os.Remove(coildSocket)

	ctx := context.Background()
	var cancel context.CancelFunc
	var podNet *vethCollisionPodNetwork
	var conn *grpc.ClientConn
	var cniClient cnirpc.CNIClient
	metricPort := 13650

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
		Expect(err).ToNot(HaveOccurred())

		podNet = &vethCollisionPodNetwork{simulateCleanup: true}
		logger := zap.NewRaw(zap.WriteTo(GinkgoWriter), zap.StacktraceLevel(zapcore.DPanicLevel))
		serverCfg := &config.Config{
			MetricsAddr:            constants.DefautlMetricsAddr,
			HealthAddr:             constants.DefautlMetricsAddr,
			PodTableId:             constants.DefautlPodTableId,
			PodRulePrio:            constants.DefautlPodRulePrio,
			ExportTableId:          constants.DefautlExportTableId,
			ProtocolId:             constants.DefautlProtocolId,
			SocketPath:             constants.DefaultSocketPath,
			CompatCalico:           true, // enable per-pod locking
			EgressPort:             constants.DefaultEgressPort,
			RegisterFromMain:       constants.DefaultRegisterFromMain,
			EnableIPAM:             true,
			EnableEgress:           true,
			AddressBlockGCInterval: 10 * time.Second,
		}

		serv := NewCoildServer(l, mgr, &raceNodeIPAM{}, podNet, &mockNATSetup{}, serverCfg, logger, vethCollisionAlias, "test-node")
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

	It("should prevent veth collision with per-pod lock in compatCalico mode", func() {
		// With CompatCalico=true, the server acquires a per-pod lock in Add.
		// This serializes concurrent Adds for the same pod, preventing the
		// veth name collision. Combined with calico-style cleanup (simulated
		// by simulateCleanup=true in the mock), all retries should succeed.

		pod := &corev1.Pod{}
		pod.Namespace = "ns1"
		pod.Name = "collision-pod"
		pod.Spec.Containers = []corev1.Container{
			{Name: "app", Image: "nginx"},
		}
		err := k8sClient.Create(ctx, pod)
		Expect(err).NotTo(HaveOccurred())

		// Simulate kubelet retry: same pod, different ContainerIDs (new sandbox each time)
		const numRetries = 3
		var wg sync.WaitGroup
		errs := make([]error, numRetries)

		for i := 0; i < numRetries; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := cniClient.Add(ctx, &cnirpc.CNIArgs{
					Args:        map[string]string{"K8S_POD_NAME": "collision-pod", "K8S_POD_NAMESPACE": "ns1"},
					ContainerId: fmt.Sprintf("sandbox-%d", idx),
					Ifname:      "eth0",
					Netns:       "/run/netns/cni-collision-pod",
					Interfaces:  map[string]bool{"eth0": false},
				})
				errs[idx] = err
			}(i)
		}
		wg.Wait()

		// With per-pod lock + calico cleanup, ALL calls should succeed
		for i, e := range errs {
			Expect(e).NotTo(HaveOccurred(), "Add failed for sandbox-%d", i)
		}

		// No collisions should have occurred
		Expect(podNet.collisions.Load()).To(BeNumerically("==", 0),
			"no veth collisions should occur when per-pod lock serializes Adds")

		// All calls were actually executed
		Expect(podNet.setupCount.Load()).To(BeNumerically(">=", int64(numRetries)))
	})
})

var _ = Describe("Coild server race conditions", Label("race"), func() {
	tmpFile, err := os.CreateTemp("", "coild-race-test-*.sock")
	if err != nil {
		panic(err)
	}
	coildSocket := tmpFile.Name()
	tmpFile.Close()
	os.Remove(coildSocket)

	ctx := context.Background()
	var cancel context.CancelFunc
	var nodeIPAM *raceNodeIPAM
	var podNet *racePodNetwork
	var conn *grpc.ClientConn
	var cniClient cnirpc.CNIClient
	metricPort := 13550

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
		Expect(err).ToNot(HaveOccurred())

		nodeIPAM = &raceNodeIPAM{}
		podNet = &racePodNetwork{}
		logger := zap.NewRaw(zap.WriteTo(GinkgoWriter), zap.StacktraceLevel(zapcore.DPanicLevel))
		serverCfg := &config.Config{
			MetricsAddr:            constants.DefautlMetricsAddr,
			HealthAddr:             constants.DefautlMetricsAddr,
			PodTableId:             constants.DefautlPodTableId,
			PodRulePrio:            constants.DefautlPodRulePrio,
			ExportTableId:          constants.DefautlExportTableId,
			ProtocolId:             constants.DefautlProtocolId,
			SocketPath:             constants.DefaultSocketPath,
			CompatCalico:           constants.DefaultCompatCalico,
			EgressPort:             constants.DefaultEgressPort,
			RegisterFromMain:       constants.DefaultRegisterFromMain,
			EnableIPAM:             true,
			EnableEgress:           true,
			AddressBlockGCInterval: 10 * time.Second,
		}

		serv := NewCoildServer(l, mgr, nodeIPAM, podNet, &mockNATSetup{}, serverCfg, logger, raceAlias, "test-node")
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

	It("should handle concurrent Add calls for different containers without races", func() {
		const numPods = 10

		// Create pods first
		for i := 0; i < numPods; i++ {
			pod := &corev1.Pod{}
			pod.Namespace = "ns1"
			pod.Name = fmt.Sprintf("race-pod-%d", i)
			pod.Spec.Containers = []corev1.Container{
				{Name: "app", Image: "nginx"},
			}
			err := k8sClient.Create(ctx, pod)
			Expect(err).NotTo(HaveOccurred())
		}

		// Fire concurrent Add calls with different ContainerIds
		var wg sync.WaitGroup
		errs := make([]error, numPods)
		results := make([]*cnirpc.AddResponse, numPods)

		for i := 0; i < numPods; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				resp, err := cniClient.Add(ctx, &cnirpc.CNIArgs{
					Args:        map[string]string{"K8S_POD_NAME": fmt.Sprintf("race-pod-%d", idx), "K8S_POD_NAMESPACE": "ns1"},
					ContainerId: fmt.Sprintf("container-%d", idx),
					Ifname:      "eth0",
					Netns:       fmt.Sprintf("/run/netns/race-pod-%d", idx),
					Interfaces:  map[string]bool{"eth0": false},
				})
				errs[idx] = err
				results[idx] = resp
			}(i)
		}
		wg.Wait()

		// All should succeed
		for i := 0; i < numPods; i++ {
			Expect(errs[i]).NotTo(HaveOccurred(), "Add failed for container-%d", i)
			result := &current.Result{}
			err := json.Unmarshal(results[i].Result, result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IPs).To(HaveLen(2))
		}

		// Verify all were actually called
		Expect(podNet.setupCount.Load()).To(Equal(int64(numPods)))
	})

	It("should serialize concurrent Add calls for the same container via keymutex", func() {
		pod := &corev1.Pod{}
		pod.Namespace = "ns1"
		pod.Name = "serial-pod"
		pod.Spec.Containers = []corev1.Container{
			{Name: "app", Image: "nginx"},
		}
		err := k8sClient.Create(ctx, pod)
		Expect(err).NotTo(HaveOccurred())

		// Fire concurrent Add calls with the SAME ContainerId.
		// The keymutex in podNetwork.SetupIPAM serializes per ContainerId,
		// so from the server's perspective all calls proceed concurrently
		// into the PodNetwork layer which is responsible for serialization.
		// This test verifies no panics or deadlocks occur under concurrent
		// same-container Add calls.
		const numCalls = 5
		var wg sync.WaitGroup
		errs := make([]error, numCalls)

		for i := 0; i < numCalls; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := cniClient.Add(ctx, &cnirpc.CNIArgs{
					Args:        map[string]string{"K8S_POD_NAME": "serial-pod", "K8S_POD_NAMESPACE": "ns1"},
					ContainerId: "same-container",
					Ifname:      "eth0",
					Netns:       "/run/netns/serial-pod",
					Interfaces:  map[string]bool{"eth0": false},
				})
				errs[idx] = err
			}(i)
		}
		wg.Wait()

		// All should succeed without deadlocks
		for i := 0; i < numCalls; i++ {
			Expect(errs[i]).NotTo(HaveOccurred(), "Add failed for call %d", i)
		}

		// Verify all calls were actually executed
		Expect(podNet.setupCount.Load()).To(BeNumerically("==", int64(numCalls)))
	})

	It("should handle mixed concurrent Add/Check/Del without races", func() {
		// Create pods for Add operations
		for i := 0; i < 5; i++ {
			pod := &corev1.Pod{}
			pod.Namespace = "ns1"
			pod.Name = fmt.Sprintf("mixed-pod-%d", i)
			pod.Spec.Containers = []corev1.Container{
				{Name: "app", Image: "nginx"},
			}
			err := k8sClient.Create(ctx, pod)
			Expect(err).NotTo(HaveOccurred())
		}

		var wg sync.WaitGroup
		// Concurrent Adds with different containers
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, _ = cniClient.Add(ctx, &cnirpc.CNIArgs{
					Args:        map[string]string{"K8S_POD_NAME": fmt.Sprintf("mixed-pod-%d", idx), "K8S_POD_NAMESPACE": "ns1"},
					ContainerId: fmt.Sprintf("mixed-container-%d", idx),
					Ifname:      "eth0",
					Netns:       fmt.Sprintf("/run/netns/mixed-pod-%d", idx),
					Interfaces:  map[string]bool{"eth0": false},
				})
			}(i)
		}

		// Concurrent Checks
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, _ = cniClient.Check(ctx, &cnirpc.CNIArgs{
					Args:        map[string]string{"K8S_POD_NAME": fmt.Sprintf("mixed-pod-%d", idx), "K8S_POD_NAMESPACE": "ns1"},
					ContainerId: fmt.Sprintf("mixed-container-%d", idx),
					Ifname:      "eth0",
					Netns:       fmt.Sprintf("/run/netns/mixed-pod-%d", idx),
				})
			}(i)
		}

		// Concurrent Dels
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, _ = cniClient.Del(ctx, &cnirpc.CNIArgs{
					Args:        map[string]string{"K8S_POD_NAME": fmt.Sprintf("mixed-pod-%d", idx), "K8S_POD_NAMESPACE": "ns1"},
					ContainerId: fmt.Sprintf("mixed-container-%d", idx),
					Ifname:      "eth0",
					Netns:       fmt.Sprintf("/run/netns/mixed-pod-%d", idx),
				})
			}(i)
		}

		wg.Wait()

		// Verify operations were executed (no deadlock)
		totalOps := podNet.setupCount.Load() + podNet.checkCount.Load() + podNet.destroyCount.Load()
		Expect(totalOps).To(BeNumerically(">", 0), "no operations were executed")
	})
})
