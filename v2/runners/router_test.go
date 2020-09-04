package runners

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/expfmt"
	ctrl "sigs.k8s.io/controller-runtime"
)

type fakeSyncer struct {
	ch chan<- map[string]nodenet.GatewayInfo
}

func (s *fakeSyncer) Sync(gis []nodenet.GatewayInfo) error {
	m := make(map[string]nodenet.GatewayInfo)
	for _, gi := range gis {
		m[gi.Gateway.String()] = gi
	}
	s.ch <- m
	return nil
}

func newIPNet(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return n
}

func createBlock(ctx context.Context, name, nodeName string, ipv4, ipv6 *net.IPNet) {
	block := &coilv2.AddressBlock{}
	block.Name = name
	block.Labels = map[string]string{constants.LabelNode: nodeName}
	if ipv4 != nil {
		s := ipv4.String()
		block.IPv4 = &s
	}
	if ipv6 != nil {
		s := ipv6.String()
		block.IPv6 = &s
	}
	err := k8sClient.Create(ctx, block)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

var _ = Describe("Router", func() {
	ctx := context.Background()
	var stopCh chan struct{}
	syncCh := make(chan struct{})
	resultCh := make(chan map[string]nodenet.GatewayInfo)

	BeforeEach(func() {
		stopCh = make(chan struct{})
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "localhost:13450",
		})
		Expect(err).ToNot(HaveOccurred())

		r := NewRouter(mgr, ctrl.Log.WithName("garbage collector"), "node1", syncCh, &fakeSyncer{ch: resultCh}, 1*time.Minute)
		err = mgr.Add(r)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			err := mgr.Start(stopCh)
			if err != nil {
				panic(err)
			}
		}()
	})

	AfterEach(func() {
		deleteAllAddressBlocks()
		close(stopCh)
		time.Sleep(10 * time.Millisecond)
	})

	It("should convert address blocks to routes", func() {
		By("synchronizing address blocks")
		// node1 is the running node, so ignored
		createBlock(ctx, "block-0", "node1", newIPNet("10.30.1.0/24"), newIPNet("fd02::0100/120"))
		// node2 does not exist, so ignored
		createBlock(ctx, "block-1", "node2", newIPNet("10.30.2.0/24"), newIPNet("fd02::0200/120"))
		// node4 has only IPv4 address
		createBlock(ctx, "block-2", "node4", newIPNet("10.30.4.0/24"), newIPNet("fd02::0400/120"))
		// node5 has only IPv6 address
		createBlock(ctx, "block-3", "node5", newIPNet("10.30.5.0/24"), newIPNet("fd02::0500/120"))
		// node6 has both IPv4 and IPv6 addresses
		createBlock(ctx, "block-4", "node6", newIPNet("10.30.6.0/24"), newIPNet("fd02::0600/120"))

		Eventually(func() error {
			syncCh <- struct{}{}
			result := <-resultCh

			if _, ok := result[net.ParseIP("10.20.30.41").String()]; ok {
				return fmt.Errorf("should not have 10.20.30.41: %v", result)
			}
			if _, ok := result[net.ParseIP("fd10::41").String()]; ok {
				return fmt.Errorf("should not have fd10::41: %v", result)
			}
			if _, ok := result[net.ParseIP("fd10::44").String()]; !ok {
				return fmt.Errorf("should have fd10::44: %v", result)
			}
			if _, ok := result[net.ParseIP("10.20.30.45").String()]; !ok {
				return fmt.Errorf("should have 10.20.30.45: %v", result)
			}
			if _, ok := result[net.ParseIP("10.20.30.46").String()]; !ok {
				return fmt.Errorf("should have 10.20.30.46: %v", result)
			}
			if _, ok := result[net.ParseIP("fd10::46").String()]; !ok {
				return fmt.Errorf("should have fd10::46: %v", result)
			}

			nets := result[net.ParseIP("fd10::44").String()].Networks
			expected := []*net.IPNet{newIPNet("fd02::0400/120")}
			if !cmp.Equal(nets, expected) {
				return fmt.Errorf("unexpected networks: %v", cmp.Diff(nets, expected))
			}

			nets = result[net.ParseIP("10.20.30.45").String()].Networks
			expected = []*net.IPNet{newIPNet("10.30.5.0/24")}
			if !cmp.Equal(nets, expected) {
				return fmt.Errorf("unexpected networks: %v", cmp.Diff(nets, expected))
			}

			return nil
		}).Should(Succeed())

		By("adding more blocks")
		createBlock(ctx, "block-5", "node6", newIPNet("10.30.7.0/24"), nil)
		createBlock(ctx, "block-6", "node6", nil, newIPNet("fd02::0800/120"))

		Eventually(func() error {
			syncCh <- struct{}{}
			result := <-resultCh

			if _, ok := result[net.ParseIP("10.20.30.41").String()]; ok {
				return fmt.Errorf("should not have 10.20.30.41: %v", result)
			}
			if _, ok := result[net.ParseIP("fd10::41").String()]; ok {
				return fmt.Errorf("should not have fd10::41: %v", result)
			}

			nets := result[net.ParseIP("10.20.30.46").String()].Networks
			expected := []*net.IPNet{newIPNet("10.30.6.0/24"), newIPNet("10.30.7.0/24")}
			if !cmp.Equal(nets, expected) {
				return fmt.Errorf("unexpected networks: %v", cmp.Diff(nets, expected))
			}

			nets = result[net.ParseIP("fd10::46").String()].Networks
			expected = []*net.IPNet{newIPNet("fd02::0600/120"), newIPNet("fd02::0800/120")}
			if !cmp.Equal(nets, expected) {
				return fmt.Errorf("unexpected networks: %v", cmp.Diff(nets, expected))
			}

			return nil
		}).Should(Succeed())

		By("checking metrics")
		resp, err := http.Get("http://localhost:13450/metrics")
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		mfs, err := (&expfmt.TextParser{}).TextToMetricFamilies(resp.Body)
		Expect(err).NotTo(HaveOccurred())

		Expect(mfs).To(HaveKey("coil_router_syncs_total"))
		mf := mfs["coil_router_syncs_total"]
		metric := findMetric(mf, map[string]string{
			"node": "node1",
		})
		Expect(metric).NotTo(BeNil())
		Expect(metric.GetCounter().GetValue()).To(BeNumerically(">=", 2))

		Expect(mfs).To(HaveKey("coil_router_routes_synced"))
		mf = mfs["coil_router_routes_synced"]
		metric = findMetric(mf, map[string]string{
			"node": "node1",
		})
		Expect(metric).NotTo(BeNil())
		Expect(metric.GetGauge().GetValue()).To(BeNumerically("==", 6))
	})
})
