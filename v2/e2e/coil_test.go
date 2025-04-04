package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/expfmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
)

var (
	enableIPv6Tests   = os.Getenv(testIPv6Key) == "true"
	enableIPAMTests   = os.Getenv(testIPAMKey) == "true"
	enableEgressTests = os.Getenv(testEgressKey) == "true"
)

var _ = Describe("coil", func() {
	if enableIPAMTests {
		Context("when the IPAM features are enabled", testIPAM)
	}
	if enableEgressTests {
		Context("when egress feature is enabled", testEgress)
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

		Eventually(func() error {
			_, err := kubectl(nil, "apply", "-f", manifest)
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("creating pods")
		kubectlSafe(nil, "apply", "-f", "manifests/httpd.yaml")
		kubectlSafe(nil, "apply", "-f", "manifests/ubuntu.yaml")
		var httpdIP, httpdNode, ubuntuNode string
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
					httpdIP = pod.Status.PodIP
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
		if enableIPv6Tests {
			testURL = fmt.Sprintf("http://[%s]:8000", httpdIP)
		} else {
			testURL = fmt.Sprintf("http://%s:8000", httpdIP)
		}
		Expect(ubuntuNode).NotTo(Equal(httpdNode))
		out := kubectlSafe(nil, "exec", "ubuntu", "--", "curl", "-s", testURL)
		Expect(string(out)).To(Equal("Hello"))
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
		var ipOpt string
		if enableIPv6Tests {
			ipOpt = "-6"
		} else {
			ipOpt = "-4"
		}
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
}

func testEgress() {
	It("should be able to run Egress pods", func() {
		By("defining Egress in the internet namespace")

		Eventually(func() error {
			_, err := kubectl(nil, "apply", "-f", "manifests/egress.yaml")
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("checking pod deployments")
		Eventually(func() int {
			depl := &appsv1.Deployment{}
			err := getResource("internet", "deployments", "egress", "", depl)
			if err != nil {
				return 0
			}
			return int(depl.Status.ReadyReplicas)
		}).Should(Equal(2))

		By("checking PDB")
		Eventually(func() error {
			pdb := &policyv1.PodDisruptionBudget{}
			err := getResource("internet", "pdb", "egress", "", pdb)
			if err != nil {
				return err
			}
			return nil
		}).Should(Succeed())

		By("defining Egress with fouSourcePortAuto in the internet namespace")

		Eventually(func() error {
			_, err := kubectl(nil, "apply", "-f", "manifests/egress-sport-auto.yaml")
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("checking pod deployments for fouSourcePortAuto")
		Eventually(func() int {
			depl := &appsv1.Deployment{}
			err := getResource("internet", "deployments", "egress-sport-auto", "", depl)
			if err != nil {
				return 0
			}
			return int(depl.Status.ReadyReplicas)
		}).Should(Equal(2))

		By("deleting Egress")
		kubectlSafe(nil, "delete", "egress", "-n", "internet", "egress")

		By("checking PDB deletion")
		Eventually(func() error {
			pdb := &policyv1.PodDisruptionBudget{}
			err := getResource("internet", "pdb", "egress", "", pdb)
			if err != nil {
				return err
			}
			return nil
		}).ShouldNot(Succeed())
	})

	It("should be able to run NAT client pods", func() {
		By("defining Egress in the internet namespace")

		Eventually(func() error {
			_, err := kubectl(nil, "apply", "-f", "manifests/egress.yaml")
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("creating a NAT client pod")
		kubectlSafe(nil, "apply", "-f", "manifests/nat-client.yaml")

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
			_, err := kubectl(nil, "apply", "-f", "manifests/egress-sport-auto.yaml")
			return err
		}).WithTimeout(5 * time.Minute).WithPolling(100 * time.Millisecond).ShouldNot(HaveOccurred())

		By("creating a NAT client pod for fouSourcePortAuto")
		kubectlSafe(nil, "apply", "-f", "manifests/nat-client-sport-auto.yaml")

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
		var fakeIP, fakeURL, ipOpt string
		if enableIPv6Tests {
			fakeIP = "2606:4700:4700::9999"
			fakeURL = fmt.Sprintf("http://[%s]", fakeIP)
			ipOpt = "-6"
		} else {
			fakeIP = "9.9.9.9"
			fakeURL = "http://" + fakeIP
			ipOpt = "-4"
		}

		By("setting a fake global address to coil-control-plane")
		_, err := runOnNode("coil-control-plane", "ip", "link", "add", "dummy-fake", "type", "dummy")
		Expect(err).NotTo(HaveOccurred())
		_, err = runOnNode("coil-control-plane", "ip", "link", "set", "dummy-fake", "up")
		Expect(err).NotTo(HaveOccurred())
		if enableIPv6Tests {
			_, err = runOnNode("coil-control-plane", "ip", "address", "add", fakeIP+"/128", "dev", "dummy-fake", "nodad")
		} else {
			_, err = runOnNode("coil-control-plane", "ip", "address", "add", fakeIP+"/32", "dev", "dummy-fake")
		}
		Expect(err).NotTo(HaveOccurred())

		natAddresses := []string{}
		if !enableIPAMTests {
			natAddresses = getNATAddresses("egress")
		}

		By("running HTTP server on coil-control-plane")
		if enableIPAMTests {
			go runOnNode("coil-control-plane", "/usr/local/bin/echotest")
		} else {
			go runOnNode("coil-control-plane", "/usr/local/bin/echotest", "--reply-remote")
		}

		time.Sleep(100 * time.Millisecond)

		By("sending and receiving HTTP request from nat-client")
		data := make([]byte, 1<<20) // 1 MiB
		testNAT(data, "nat-client", fakeURL, natAddresses, enableIPAMTests)

		By("running the same test 100 times")
		for i := 0; i < 100; i++ {
			time.Sleep(1 * time.Millisecond)
			testNAT(data, "nat-client", fakeURL, natAddresses, enableIPAMTests)
		}

		natAddresses = []string{}
		if !enableIPAMTests {
			natAddresses = getNATAddresses("egress-sport-auto")
		}

		By("sending and receiving HTTP request from nat-client-sport-auto")
		data = make([]byte, 1<<20) // 1 MiB
		testNAT(data, "nat-client-sport-auto", fakeURL, natAddresses, enableIPAMTests)

		By("running the same test 100 times with nat-client-sport-auto")
		for i := 0; i < 100; i++ {
			time.Sleep(1 * time.Millisecond)
			testNAT(data, "nat-client-sport-auto", fakeURL, natAddresses, enableIPAMTests)
		}

		By("creating a dummy pod don't use egress")
		// dummy pod must be created after creating a net-client pod
		kubectlSafe(nil, "apply", "-f", "manifests/dummy_pod.yaml")

		By("updating Egress in the internet namespace")
		kubectlSafe(nil, "apply", "-f", "manifests/egress-updated.yaml")

		By("waiting for the Egress rollout restart")
		kubectlSafe(nil, "-n", "internet", "rollout", "status", "deploy", "egress")

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

		Eventually(func() error {
			out, err := kubectl(nil, "exec", "nat-client", "--", "ip", ipOpt, "-d", "-j", "link", "show")
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
				if strings.HasPrefix(l.Ifname, "fou") && l.Ifname != "fou-dummy" {
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

			out, err = kubectl(nil, "exec", "nat-client", "--", "ip", ipOpt, "-j", "route", "show", "table", "118")
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
			if routes[0].Dst != fakeIP {
				return fmt.Errorf("route Dst is not %s actual: %s", fakeIP, routes[0].Dst)
			}

			return nil
		}).Should(Succeed())

		By("confirming that the fou device must be one in dummy_pod")
		out, err := kubectl(nil, "exec", "dummy", "--", "ip", "-j", "link", "show")
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
		Expect(fouCount).To(Equal(1))

		By("sending and receiving HTTP request from nat-client")

		natAddresses = []string{}
		if !enableIPAMTests {
			natAddresses = getNATAddresses("egress")
		}

		data = make([]byte, 1<<20) // 1 MiB
		testNAT(data, "nat-client", fakeURL, natAddresses, enableIPAMTests)

		By("running the same test 100 times")
		for i := 0; i < 100; i++ {
			time.Sleep(1 * time.Millisecond)
			testNAT(data, "nat-client", fakeURL, natAddresses, enableIPAMTests)
		}
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
	eps := &corev1.Endpoints{}
	err := getResource("internet", "endpoints", name, "", eps)
	Expect(err).ToNot(HaveOccurred())

	natAddresses := []string{}
	for _, s := range eps.Subsets {
		for _, a := range s.Addresses {
			natAddresses = append(natAddresses, a.IP)
		}
	}
	return natAddresses
}
