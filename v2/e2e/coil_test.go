package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/expfmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	policyv1 "k8s.io/api/policy/v1"
)

var (
	enableIPv4Tests   = true
	enableIPv6Tests   = false
	enableIPAMTests   = true
	enableEgressTests = true
)

func ParseEnv() {
	enableIPv4Tests = ParseBool(testIPv4Key)
	enableIPv6Tests = ParseBool(testIPv6Key)
	enableIPAMTests = ParseBool(testIPAMKey)
	enableEgressTests = ParseBool(testEgressKey)
}

func ParseBool(key string) bool {
	value, _ := strconv.ParseBool(os.Getenv(key))
	return value
}

var _ = Describe("coil", func() {
	ParseEnv()
	if enableIPAMTests {
		Context("when the IPAM features are enabled", Ordered, testIPAM)
	}
	if enableEgressTests {
		Context("when egress feature is enabled", Ordered, testEgress)
	}
	Context("when coild is deployed", testCoild)
})

func testIPAM() {
	It("should run health probe servers", func() {
		By("checking all pods get ready")
		Eventually(func() error {
			pods := &corev1.PodList{}
			err := getResource("kube-system", "pods", "", "app.kubernetes.io/name=coil", pods)
			if err != nil {
				return err
			}

			if len(pods.Items) == 0 {
				return errors.New("bug")
			}

		OUTER:
			for _, pod := range pods.Items {
				for _, cond := range pod.Status.Conditions {
					if cond.Type != corev1.PodReady {
						continue
					}
					if cond.Status == corev1.ConditionTrue {
						continue OUTER
					}
				}
				return fmt.Errorf("pod is not ready: %s", pod.Name)
			}

			return nil
		}).Should(Succeed())
	})

	// This series of tests confirms the following things:
	// - coil can call coild gRPC method appropriately
	// - coild runs gRPC server
	// - coil-ipam-controller can process BlockRequest
	// - coil-router setups the kernel routing table
	It("should allow pods on different nodes to communicate", func() {
		By("creating the default pool")
		manifest := "manifests/default_pool.yaml"
		if enableIPv6Tests {
			manifest = "manifests/default_pool_v6.yaml"
		}

		if enableIPv4Tests && enableIPv6Tests {
			manifest = "manifests/default_pool_dualstack.yaml"
		}

		Eventually(func() error {
			_, err := kubectl(nil, "apply", "-f", manifest)
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("creating pods")
		kubectlSafe(nil, "apply", "-f", "manifests/httpd.yaml")
		kubectlSafe(nil, "apply", "-f", "manifests/ubuntu.yaml")
		var httpdNode, ubuntuNode string
		httpdIP := []corev1.PodIP{}
		Eventually(func() error {
			pods := &corev1.PodList{}
			err := getResource("default", "pods", "", "", pods)
			if err != nil {
				return err
			}

			if len(pods.Items) != 2 {
				return fmt.Errorf("wrong number of pods: %d", len(pods.Items))
			}

		OUTER:
			for _, pod := range pods.Items {
				if pod.Name == "httpd" {
					httpdIP = pod.Status.PodIPs
					httpdNode = pod.Spec.NodeName
				} else {
					ubuntuNode = pod.Spec.NodeName
				}
				for _, cond := range pod.Status.Conditions {
					if cond.Type != corev1.PodReady {
						continue
					}
					if cond.Status != corev1.ConditionTrue {
						return fmt.Errorf("pod is not ready: %s", pod.Name)
					}
					continue OUTER
				}
				return fmt.Errorf("pod is not ready: %s", pod.Name)
			}

			return nil
		}).Should(Succeed())

		By("checking communication between pods on different nodes")
		var testURL string

		for i := range httpdIP {
			if enableIPv6Tests && net.ParseIP(httpdIP[i].IP).To16() != nil {
				testURL = fmt.Sprintf("http://[%s]:8000", httpdIP[i].IP)
			}
			if enableIPv4Tests && net.ParseIP(httpdIP[i].IP).To4() != nil {
				testURL = fmt.Sprintf("http://%s:8000", httpdIP[i].IP)
			}
			if testURL != "" {
				Expect(ubuntuNode).NotTo(Equal(httpdNode))
				out := kubectlSafe(nil, "exec", "ubuntu", "--", "curl", "-s", testURL)
				Expect(string(out)).To(Equal("Hello"))
			}
		}

	})

	It("should persist IPAM status between coild restarts", func() {
		By("removing coild pods")
		kubectlSafe(nil, "-n", "kube-system", "delete", "pods", "-l", "app.kubernetes.io/component=coild")
		Eventually(func() error {
			ds := &appsv1.DaemonSet{}
			err := getResource("kube-system", "daemonsets", "coild", "", ds)
			if err != nil {
				return err
			}
			if ds.Status.NumberReady == 4 {
				return errors.New("not yet deleted")
			}
			return nil
		}).Should(Succeed())
		Eventually(func() error {
			ds := &appsv1.DaemonSet{}
			err := getResource("kube-system", "daemonsets", "coild", "", ds)
			if err != nil {
				return err
			}
			if ds.Status.NumberReady != 4 {
				return errors.New("not yet recreated")
			}
			return nil
		})

		By("checking AddressBlock")
		// This needs to be Eventually because sometimes coild has extra AddressBlock.
		// This happens, for example, when gRPC request is timed-out but the request
		// succeeds and a new block is allocated for the node.
		Eventually(func() error {
			abl := &coilv2.AddressBlockList{}
			err := getResource("", "addressblocks", "", "coil.cybozu.com/node=coil-worker", abl)
			if err != nil {
				return err
			}
			if len(abl.Items) != 1 {
				return fmt.Errorf("# of AddressBlocks for coil-worker is wrong: %d", len(abl.Items))
			}

			abl = &coilv2.AddressBlockList{}
			err = getResource("", "addressblocks", "", "coil.cybozu.com/node=coil-worker2", abl)
			if err != nil {
				return err
			}
			if len(abl.Items) != 1 {
				return fmt.Errorf("# of AddressBlocks for coil-worker2 is wrong: %d", len(abl.Items))
			}
			return nil
		}).Should(Succeed())

		By("confirming a new AddressBlock is allocated for a new pod")
		kubectlSafe(nil, "apply", "-f", "manifests/another_httpd.yaml")
		Eventually(func() int {
			abl := &coilv2.AddressBlockList{}
			err := getResource("", "addressblocks", "", "", abl)
			if err != nil {
				return 0
			}
			count := 0
			for _, b := range abl.Items {
				switch b.Labels["coil.cybozu.com/node"] {
				case "coil-worker", "coil-worker2":
					count++
				}
			}
			return count
		}).Should(Equal(3))
	})

	It("should export routes to routing table 119", func() {
		var ipOpts []string
		if enableIPv4Tests {
			ipOpts = append(ipOpts, "-4")
		}
		if enableIPv6Tests {
			ipOpts = append(ipOpts, "-6")
		}

		for _, ipOpt := range ipOpts {
			out, err := runOnNode("coil-worker", "ip", ipOpt, "-j", "route", "show", "table", "119")
			Expect(err).ShouldNot(HaveOccurred())

			type route struct {
				Dst      string `json:"dst"`
				Dev      string `json:"dev"`
				Protocol string `json:"protocol"`
			}
			var routes []route
			err = json.Unmarshal(out, &routes)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(routes).NotTo(BeEmpty())
		}

	})

	It("should free unused AddressBlocks", func() {
		By("deleting pods")
		kubectlSafe(nil, "delete", "pods", "--all")

		By("counting AddressBlocks")
		Eventually(func() error {
			abl := &coilv2.AddressBlockList{}
			err := getResource("", "addressblocks", "", "", abl)
			if err != nil {
				return err
			}
			for _, b := range abl.Items {
				switch b.Labels["coil.cybozu.com/node"] {
				case "coil-worker", "coil-worker2":
					return fmt.Errorf("AddressBlock not deleted: %+v", b)
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should export metrics", func() {
		By("checking port 9388 for coil-router")
		out, err := runOnNode("coil-worker", "curl", "-sf", "http://localhost:9388/metrics")
		Expect(err).ShouldNot(HaveOccurred())

		mfs, err := (&expfmt.TextParser{}).TextToMetricFamilies(bytes.NewReader(out))
		Expect(err).NotTo(HaveOccurred())
		Expect(mfs).NotTo(BeEmpty())

	})

	It("should delete address pool", func() {
		By("creating a dummy address pool")
		_, err := kubectl(nil, "apply", "-f", "manifests/dummy_pool.yaml")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			ap := coilv2.AddressPool{}
			out, err := kubectl(nil, "get", "addresspool", "dummy", "-o", "json")
			if err != nil {
				return err
			}
			if err := json.Unmarshal(out, &ap); err != nil {
				return err
			}
			return nil
		}).Should(Succeed())

		By("deleting the dummy address pool")
		_, err = kubectl(nil, "delete", "-f", "manifests/dummy_pool.yaml")
		Expect(err).NotTo(HaveOccurred())

		Consistently(func() error {
			apList := coilv2.AddressPoolList{}
			out, err := kubectl(nil, "get", "addresspool", "-o", "json")
			if err != nil {
				return err
			}
			if err := json.Unmarshal(out, &apList); err != nil {
				return err
			}
			if len(apList.Items) == 1 {
				return nil
			}
			return fmt.Errorf("the number of AddressPool must be 1")
		}).Should(Succeed())
	})

	It("should cleanup network state", func() {
		By("ensuring complete cleanup after IPAM tests")

		kubectlSafe(nil, "delete", "addressblock", "--all", "--ignore-not-found=true")
		kubectlSafe(nil, "delete", "blockrequest", "--all", "--ignore-not-found=true")

		runOnNode("coil-worker", "ip", "route", "flush", "table", "119")
		runOnNode("coil-worker2", "ip", "route", "flush", "table", "119")
		runOnNode("coil-control-plane", "ip", "route", "flush", "table", "all")
	})
}

type egressTestCase struct {
	name                    string
	egressManifest          string
	egressSportManifest     string
	egressUpdatedManifest   string
	dummyPodManifest        string
	natClient               string
	natClientSportAuto      string
	egressDeploymentName    string
	sportAutoDeploymentName string
}

func testEgress() {
	testCases := []egressTestCase{
		{
			name:                    "iptables",
			egressManifest:          "manifests/egress.yaml",
			egressSportManifest:     "manifests/egress-sport-auto.yaml",
			egressUpdatedManifest:   "manifests/egress-updated.yaml",
			dummyPodManifest:        "manifests/dummy_pod.yaml",
			natClient:               "manifests/nat-client.yaml",
			natClientSportAuto:      "manifests/nat-client-sport-auto.yaml",
			egressDeploymentName:    "egress",
			sportAutoDeploymentName: "egress-sport-auto",
		},
		{
			name:                    "nftables",
			egressManifest:          "manifests/egress-nft.yaml",
			egressSportManifest:     "manifests/egress-sport-auto-nft.yaml",
			egressUpdatedManifest:   "manifests/egress-updated-nft.yaml",
			dummyPodManifest:        "manifests/dummy_pod-nft.yaml",
			natClient:               "manifests/nat-client-nft.yaml",
			natClientSportAuto:      "manifests/nat-client-sport-auto-nft.yaml",
			egressDeploymentName:    "egress-nft",
			sportAutoDeploymentName: "egress-sport-auto-nft",
		},
	}

	for _, tc := range testCases {
		Context(fmt.Sprintf("with %s", tc.name), func() {
			testEgressWithBackend(tc)
		})
	}
}

func testEgressWithBackend(tc egressTestCase) {
	It("should be able to run Egress pods", func() {
		By(fmt.Sprintf("defining Egress in the internet namespace using %s", tc.name))

		Eventually(func() error {
			_, err := kubectl(nil, "apply", "-f", tc.egressManifest)
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("checking pod deployments")
		Eventually(func() int {
			depl := &appsv1.Deployment{}
			err := getResource("internet", "deployments", tc.egressDeploymentName, "", depl)
			if err != nil {
				return 0
			}
			return int(depl.Status.ReadyReplicas)
		}).Should(Equal(2))

		By("checking PDB")
		Eventually(func() error {
			pdb := &policyv1.PodDisruptionBudget{}
			err := getResource("internet", "pdb", tc.egressDeploymentName, "", pdb)
			if err != nil {
				return err
			}
			return nil
		}).Should(Succeed())

		By("defining Egress with fouSourcePortAuto in the internet namespace")

		Eventually(func() error {
			_, err := kubectl(nil, "apply", "-f", tc.egressSportManifest)
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("checking pod deployments for fouSourcePortAuto")
		Eventually(func() int {
			depl := &appsv1.Deployment{}
			err := getResource("internet", "deployments", tc.sportAutoDeploymentName, "", depl)
			if err != nil {
				return 0
			}
			return int(depl.Status.ReadyReplicas)
		}).Should(Equal(2))

		By("deleting Egress")
		kubectlSafe(nil, "delete", "egress", "-n", "internet", tc.egressDeploymentName)

		By("checking PDB deletion")
		Eventually(func() error {
			pdb := &policyv1.PodDisruptionBudget{}
			err := getResource("internet", "pdb", tc.egressDeploymentName, "", pdb)
			if err != nil {
				return err
			}
			return nil
		}).ShouldNot(Succeed())

		// Clean up for next test
		By("cleaning up egress-sport-auto")
		kubectlSafe(nil, "delete", "egress", "-n", "internet", tc.sportAutoDeploymentName)
	})

	It("should be able to run NAT client pods", func() {
		By(fmt.Sprintf("defining Egress in the internet namespace using %s", tc.name))

		Eventually(func() error {
			_, err := kubectl(nil, "apply", "-f", tc.egressManifest)
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("creating a NAT client pod")
		kubectlSafe(nil, "apply", "-f", tc.natClient)

		By("checking the pod status")
		Eventually(func() error {
			pod := &corev1.Pod{}
			err := getResource("default", "pods", "nat-client", "", pod)
			if err != nil {
				return err
			}
			if len(pod.Status.ContainerStatuses) == 0 {
				return errors.New("no container status")
			}
			cst := pod.Status.ContainerStatuses[0]
			if !cst.Ready {
				return errors.New("container is not ready")
			}
			return nil
		}).Should(Succeed())

		By("defining Egress with fouSourcePortAuto in the internet namespace")

		Eventually(func() error {
			_, err := kubectl(nil, "apply", "-f", tc.egressSportManifest)
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("creating a NAT client pod for fouSourcePortAuto")
		kubectlSafe(nil, "apply", "-f", tc.natClientSportAuto)

		By("checking the pod status for fouSourcePortAuto")
		Eventually(func() error {
			pod := &corev1.Pod{}
			err := getResource("default", "pods", "nat-client-sport-auto", "", pod)
			if err != nil {
				return err
			}
			if len(pod.Status.ContainerStatuses) == 0 {
				return errors.New("no container status")
			}
			cst := pod.Status.ContainerStatuses[0]
			if !cst.Ready {
				return errors.New("container is not ready")
			}
			return nil
		}).Should(Succeed())
	})

	It("should allow NAT traffic over foo-over-udp tunnel", func() {
		type options struct {
			fakeIP  string
			fakeURL string
			ipOpt   string
		}

		opts := []options{}

		// var fakeIP, fakeURL, ipOpt string
		if enableIPv6Tests {
			fakeIP := "2606:4700:4700::9999"
			opts = append(opts, options{fakeIP, fmt.Sprintf("http://[%s]", fakeIP), "-6"})
		}
		if enableIPv4Tests {
			fakeIP := "9.9.9.9"
			opts = append(opts, options{fakeIP, "http://" + fakeIP, "-4"})
		}

		By("setting a fake global address to coil-control-plane")
		// Check if dummy-fake already exists and skip if it does
		_, err := runOnNode("coil-control-plane", "ip", "link", "show", "dummy-fake")
		if err != nil {
			_, err = runOnNode("coil-control-plane", "ip", "link", "add", "dummy-fake", "type", "dummy")
			Expect(err).NotTo(HaveOccurred())
			_, err = runOnNode("coil-control-plane", "ip", "link", "set", "dummy-fake", "up")
			Expect(err).NotTo(HaveOccurred())

			for _, o := range opts {
				if strings.Contains(o.ipOpt, "4") {
					_, err = runOnNode("coil-control-plane", "ip", "address", "add", o.fakeIP+"/32", "dev", "dummy-fake")
				} else {
					_, err = runOnNode("coil-control-plane", "ip", "address", "add", o.fakeIP+"/128", "dev", "dummy-fake", "nodad")
				}
				// Ignore error if address already exists
				if err != nil && !strings.Contains(err.Error(), "File exists") {
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}

		natAddresses := []string{}
		if !enableIPAMTests {
			natAddresses = getNATAddresses(tc.egressDeploymentName)
		}

		By("running HTTP server on coil-control-plane")
		if enableIPAMTests {
			go runOnNode("coil-control-plane", "/usr/local/bin/echotest")
		} else {
			go runOnNode("coil-control-plane", "/usr/local/bin/echotest", "--reply-remote")
		}

		time.Sleep(100 * time.Millisecond)

		for _, o := range opts {
			natAddressesFiltered := []string{}
			for _, a := range natAddresses {
				if strings.Contains(o.ipOpt, "4") && net.ParseIP(a).To4() != nil {
					natAddressesFiltered = append(natAddressesFiltered, a)
				}
				if strings.Contains(o.ipOpt, "6") && net.ParseIP(a).To16() != nil {
					natAddressesFiltered = append(natAddressesFiltered, a)
				}
			}

			By(fmt.Sprintf("sending and receiving HTTP request from nat-client-sport-auto (%s): %s", tc.name, o.fakeURL))
			data := make([]byte, 1<<20) // 1 MiB
			testNAT(data, "nat-client", o.fakeURL, natAddressesFiltered, enableIPAMTests)

			By("running the same test 100 times")
			for i := 0; i < 100; i++ {
				time.Sleep(1 * time.Millisecond)
				testNAT(data, "nat-client", o.fakeURL, natAddressesFiltered, enableIPAMTests)
			}
		}

		natAddresses = []string{}
		if !enableIPAMTests {
			natAddresses = getNATAddresses(tc.sportAutoDeploymentName)
		}

		for _, o := range opts {
			natAddressesFiltered := []string{}
			for _, a := range natAddresses {
				if strings.Contains(o.ipOpt, "4") && net.ParseIP(a).To4() != nil {
					natAddressesFiltered = append(natAddressesFiltered, a)
				}
				if strings.Contains(o.ipOpt, "6") && net.ParseIP(a).To16() != nil {
					natAddressesFiltered = append(natAddressesFiltered, a)
				}
			}

			By("sending and receiving HTTP request from nat-client-sport-auto: " + o.fakeURL)
			data := make([]byte, 1<<20) // 1 MiB
			testNAT(data, "nat-client-sport-auto", o.fakeURL, natAddressesFiltered, enableIPAMTests)

			By("running the same test 100 times with nat-client-sport-auto")
			for i := 0; i < 100; i++ {
				time.Sleep(1 * time.Millisecond)
				testNAT(data, "nat-client-sport-auto", o.fakeURL, natAddressesFiltered, enableIPAMTests)
			}
		}

		By("creating a dummy pod don't use egress")
		// dummy pod must be created after creating a net-client pod
		kubectlSafe(nil, "apply", "-f", tc.dummyPodManifest)

		By(fmt.Sprintf("updating Egress in the internet namespace using %s", tc.name))
		kubectlSafe(nil, "apply", "-f", tc.egressUpdatedManifest)

		By("waiting for the Egress rollout restart")
		kubectlSafe(nil, "-n", "internet", "rollout", "status", "deploy", tc.egressDeploymentName)

		By("checking the NAT setup in the nat-client Pod")
		type link struct {
			Ifname   string `json:"ifname"`
			Linkinfo struct {
				InfoKind string `json:"info_kind"`
				InfoData struct {
					Proto    string `json:"proto"`
					Remote   string `json:"remote"`
					Local    string `json:"local"`
					TTL      int    `json:"ttl"`
					Pmtudisc bool   `json:"pmtudisc"`
					Encap    struct {
						Type    string `json:"type"`
						Sport   int    `json:"sport"`
						Dport   int    `json:"dport"`
						Csum    bool   `json:"csum"`
						Csum6   bool   `json:"csum6"`
						Remcsum bool   `json:"remcsum"`
					} `json:"encap"`
				} `json:"info_data"`
			} `json:"linkinfo"`
		}

		type route struct {
			Dst      string `json:"dst"`
			Dev      string `json:"dev"`
			Protocol string `json:"protocol"`
		}

		for _, o := range opts {
			Eventually(func() error {
				out, err := kubectl(nil, "exec", "nat-client", "--", "ip", o.ipOpt, "-d", "-j", "link", "show")
				if err != nil {
					return err
				}
				var links []link
				err = json.Unmarshal(out, &links)
				if err != nil {
					return err
				}
				var found bool
				for _, l := range links {
					if strings.HasPrefix(l.Ifname, "fou"+strings.ReplaceAll(o.ipOpt, "-", "")) && l.Ifname != "fou-dummy" {
						if l.Linkinfo.InfoData.Encap.Sport != 0 {
							return fmt.Errorf("encap sport is not auto actual: %d", l.Linkinfo.InfoData.Encap.Sport)
						}
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("fou tunnel device not found")
				}

				out, err = kubectl(nil, "exec", "nat-client", "--", "ip", o.ipOpt, "-j", "route", "show", "table", "118")
				if err != nil {
					return err
				}
				var routes []route
				err = json.Unmarshal(out, &routes)
				if err != nil {
					return err
				}
				if len(routes) != 1 {
					return fmt.Errorf("routes length is not 1 actual: %d", len(routes))
				}
				if routes[0].Dst != o.fakeIP {
					return fmt.Errorf("route Dst is not %s actual: %s", o.fakeIP, routes[0].Dst)
				}

				return nil
			}).Should(Succeed())
		}

		By("confirming that the exact number of fou devices are present in dummy_pod")
		out, err := kubectl(nil, "exec", "dummy", "-c", "ubuntu", "--", "ip", "-j", "link", "show")
		Expect(err).NotTo(HaveOccurred())
		var dummyPodLinks []link
		err = json.Unmarshal(out, &dummyPodLinks)
		Expect(err).NotTo(HaveOccurred())
		fouCount := 0
		for _, l := range dummyPodLinks {
			if strings.HasPrefix(l.Ifname, "fou") && l.Ifname != "fou-dummy" {
				fouCount += 1
			}
		}
		expectedFouCount := 1
		if enableIPv4Tests && enableIPv6Tests {
			expectedFouCount = 2
		}
		Expect(fouCount).To(Equal(expectedFouCount))

		natAddresses = []string{}
		if !enableIPAMTests {
			natAddresses = getNATAddresses(tc.egressDeploymentName)
		}

		for _, o := range opts {
			natAddressesFiltered := []string{}
			for _, a := range natAddresses {
				if strings.Contains(o.ipOpt, "4") && net.ParseIP(a).To4() != nil {
					natAddressesFiltered = append(natAddressesFiltered, a)
				}
				if strings.Contains(o.ipOpt, "6") && net.ParseIP(a).To16() != nil {
					natAddressesFiltered = append(natAddressesFiltered, a)
				}
			}

			By(fmt.Sprintf("sending and receiving HTTP request from nat-client (%s): %s", tc.name, o.fakeURL))
			data := make([]byte, 1<<20) // 1 MiB
			testNAT(data, "nat-client", o.fakeURL, natAddressesFiltered, enableIPAMTests)

			By("running the same test 100 times")
			for i := 0; i < 100; i++ {
				time.Sleep(1 * time.Millisecond)
				testNAT(data, "nat-client", o.fakeURL, natAddressesFiltered, enableIPAMTests)
			}
		}

		// Clean up at the end
		By("cleaning up resources")
		kubectlSafe(nil, "delete", "pod", "nat-client", "--ignore-not-found=true")
		kubectlSafe(nil, "delete", "pod", "nat-client-sport-auto", "--ignore-not-found=true")
		kubectlSafe(nil, "delete", "pod", "dummy", "--ignore-not-found=true")
		kubectlSafe(nil, "delete", "egress", "-n", "internet", tc.egressDeploymentName, "--ignore-not-found=true")
		kubectlSafe(nil, "delete", "egress", "-n", "internet", tc.sportAutoDeploymentName, "--ignore-not-found=true")

		By("cleaning up dummy-fake network interface")
		runOnNode("coil-control-plane", "ip", "link", "delete", "dummy-fake")
		runOnNode("coil-control-plane", "ip", "route", "flush", "table", "all")

		By("ensuring complete cleanup between test cases")
		kubectlSafe(nil, "delete", "addressblock", "--all", "--ignore-not-found=true")
		kubectlSafe(nil, "delete", "blockrequest", "--all", "--ignore-not-found=true")
	})
}

func testCoild() {
	It("should export metrics", func() {
		By("checking port 9384 for coild")
		out, err := runOnNode("coil-worker", "curl", "-sf", "http://localhost:9384/metrics")
		Expect(err).ShouldNot(HaveOccurred())

		mfs, err := (&expfmt.TextParser{}).TextToMetricFamilies(bytes.NewReader(out))
		Expect(err).NotTo(HaveOccurred())
		Expect(mfs).NotTo(BeEmpty())
	})
}

func testNAT(data []byte, clientPod, fakeURL string, natAddresses []string, ipamEnabled bool) {
	resp := kubectlSafe(data, "exec", "-i", clientPod, "--", "curl", "-sf", "-T", "-", fakeURL)

	if !ipamEnabled {
		respStr := string(resp)
		idx := strings.Index(respStr, "|")
		ipAddr := respStr[:idx]
		resp = []byte(respStr[idx+1:])
		Expect(natAddresses).To(ContainElement(ipAddr))
	}
	Expect(resp).To(HaveLen(1 << 20))
}

func getNATAddresses(name string) []string {
	svc := &corev1.Service{}
	epslices := &discoveryv1.EndpointSliceList{}
	natAddresses := []string{}

	err := getResource("internet", "service", name, "", svc)
	Expect(err).ToNot(HaveOccurred())
	err = getResource("internet", "endpointslices", "", "", epslices)
	Expect(err).ToNot(HaveOccurred())

	for _, eps := range epslices.Items {
		for _, or := range eps.ObjectMeta.OwnerReferences {
			if or.UID == svc.UID {
				for _, ep := range eps.Endpoints {
					natAddresses = append(natAddresses, ep.Addresses...)
				}
				break
			}
		}
	}

	return natAddresses
}
