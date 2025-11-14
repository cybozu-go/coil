package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/expfmt"
	"github.com/vishvananda/netlink"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	policyv1 "k8s.io/api/policy/v1"
)

var (
	enableIPv4Tests       = true
	enableIPv6Tests       = false
	enableIPAMTests       = true
	enableEgressTests     = true
	enableOriginatingOnly = false
)

const networkInterfaceKey = "NETWORK_INTERFACE"

func ParseEnv() {
	ParseBool(testIPv4Key, &enableIPv4Tests)
	ParseBool(testIPv6Key, &enableIPv6Tests)
	ParseBool(testIPAMKey, &enableIPAMTests)
	ParseBool(testEgressKey, &enableEgressTests)
	ParseBool(testOriginatingOnlyKey, &enableOriginatingOnly)
}

func ParseBool(key string, target *bool) {
	if value, exists := os.LookupEnv(key); exists {
		*target, _ = strconv.ParseBool(value)
	}
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
		_, err := runOnNode("coil-control-plane", "ip", "link", "add", "dummy-fake", "type", "dummy")
		Expect(err).NotTo(HaveOccurred())
		_, err = runOnNode("coil-control-plane", "ip", "link", "set", "dummy-fake", "up")
		Expect(err).NotTo(HaveOccurred())

		for _, o := range opts {
			if strings.Contains(o.ipOpt, "4") {
				_, err = runOnNode("coil-control-plane", "ip", "address", "add", o.fakeIP+"/32", "dev", "dummy-fake")
			} else {
				_, err = runOnNode("coil-control-plane", "ip", "address", "add", o.fakeIP+"/128", "dev", "dummy-fake", "nodad")
			}
			Expect(err).NotTo(HaveOccurred())
		}

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

			By("sending and receiving HTTP request from nat-client: " + o.fakeURL)
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
			natAddresses = getNATAddresses("egress-sport-auto")
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
		expectedFouCount := 1
		if enableIPv4Tests && enableIPv6Tests {
			expectedFouCount = 2
		}
		Expect(fouCount).To(Equal(expectedFouCount))

		natAddresses = []string{}
		if !enableIPAMTests {
			natAddresses = getNATAddresses("egress")
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

			By("sending and receiving HTTP request from nat-client: " + o.fakeURL)
			data := make([]byte, 1<<20) // 1 MiB
			testNAT(data, "nat-client", o.fakeURL, natAddressesFiltered, enableIPAMTests)

			By("running the same test 100 times")
			for i := 0; i < 100; i++ {
				time.Sleep(1 * time.Millisecond)
				testNAT(data, "nat-client", o.fakeURL, natAddressesFiltered, enableIPAMTests)
			}
		}
	})

	It(func(originatingOnly bool) string {
		if originatingOnly {
			return "should be able to provide ingress traffic"
		}
		return "should not be able to provide ingress traffic"
	}(enableOriginatingOnly), func() {
		By("starting echo-server on the node")
		port := "12345"
		go runOnNode("coil-control-plane", "/usr/local/bin/echotest", "-port", port, "-reply-remote", "-no-separator")

		curDir, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())

		tmpDir, err := os.MkdirTemp(curDir, "tmp*")
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		}()

		By("deploying egress")
		egressData := prepareEgressData(port, fmt.Sprintf("originatingonly-%t", enableOriginatingOnly))

		deployEgress(egressData, curDir, tmpDir)

		By("deploying egress client")
		pod := deployClient(egressData, curDir, tmpDir)

		if enableOriginatingOnly {
			if enableIPv4Tests {
				By("testing IPv4 egress connectivity")
				Expect(checkEgressConnection(egressData.IPv4Addr, pod, egressData.Port)).To(Succeed())
			}

			if enableIPv6Tests {
				By("testing IPv6 egress connectivity")
				Expect(checkEgressConnection(egressData.IPv6Addr, pod, egressData.Port)).To(Succeed())
			}

			By("testing ingress connectivity")
			errs := checkIngressConnections(egressData)
			Expect(errs).To(BeEmpty())
		} else {
			expectedErr := 0
			if enableIPv4Tests {
				expectedErr++
				By("testing IPv4 egress connectivity")
				Expect(checkEgressConnection(egressData.IPv4Addr, pod, egressData.Port)).To(Succeed())
			}

			if enableIPv6Tests {
				expectedErr++
				By("testing IPv6 egress connectivity")
				Expect(checkEgressConnection(egressData.IPv6Addr, pod, egressData.Port)).To(Succeed())
			}

			By("testing ingress connectivity")
			errs := checkIngressConnections(egressData)
			Expect(len(errs)).To(Equal(expectedErr))
		}
	})
}

// egressTemplateData is used to generate test egress via template.
type egressTemplateData struct {
	IPv4Addr   string // egress IPv4 network
	IPv6Addr   string // egress IPv6 network
	PodName    string // name of the Pod that will use the generated egress (egress client)
	EgressName string // name of the created egress object
	Port       string // port on which echotest server will serve
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
	resp := kubectlSafe(data, "exec", "-i", clientPod, "--", "curl", "--max-time", "5", "-sf", "-T", "-", fakeURL)

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

func getLocalIP(ifName string, family int) (*net.IP, *net.IPNet, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, nil, fmt.Errorf("netlink: failed to list links: %w", err)
	}

	for _, link := range links {
		if strings.Contains(link.Attrs().Name, ifName) {
			ip, ipnet, err := getNetwork(link, family)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get address: %w", err)
			}
			if ip == nil {
				return nil, nil, fmt.Errorf("failed to find address on the interface %q", ifName)
			}
			return ip, ipnet, nil
		}
	}

	return nil, nil, fmt.Errorf("netlink: failed to find suitable addresses: %w", err)
}

func getNetwork(link netlink.Link, family int) (*net.IP, *net.IPNet, error) {
	addrs, err := netlink.AddrList(link, family)
	if err != nil {
		return nil, nil, fmt.Errorf("netlink: failed to get addresses for link %q: %w", link.Attrs().Name, err)
	}
	if len(addrs) > 0 {
		for _, a := range addrs {
			if a.Scope == int(netlink.SCOPE_UNIVERSE) {
				ip, cidr, err := net.ParseCIDR(a.IPNet.String())
				if err != nil {
					return nil, nil, fmt.Errorf("failed to parse CIDR: %w", err)
				}
				return &ip, cidr, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("netlink: failed to find suitable addresses in link %q: %w", link.Attrs().Name, err)
}

func checkPodIPs(ips []corev1.PodIP, addr string) bool {
	for _, ip := range ips {
		if ip.IP == addr {
			return true
		}
	}
	return false
}

func checkEgressConnection(address string, pod *corev1.Pod, port string) error {
	ip, _, err := net.ParseCIDR(address)
	if err != nil {
		return fmt.Errorf("failed to parse address %q: %w", address, err)
	}
	if ip == nil {
		return fmt.Errorf("failed to parse address %q", address)
	}

	isV6 := !(ip.To4() != nil)
	separator := "."
	if isV6 {
		separator = ":"
	}

	addr := strings.Split(address, separator)

	By("get node's IP addresses")
	var nodeIP string
	command := fmt.Sprintf("ip -j addr show dev eth0 | jq '.[].addr_info[] | .local' | grep %s", addr[0])
	nodeByte, err := runOnNode("coil-control-plane", "bash", "-c", command)
	Expect(err).ToNot(HaveOccurred())
	nodeIP = strings.ReplaceAll(strings.Trim(string(nodeByte), " \n"), "\"", "")

	By("test egress connection to IP " + nodeIP)
	if !(ip.To4() != nil) {
		nodeIP = fmt.Sprintf("[%s]", nodeIP) // IPv6 address
	}
	result := runOnPod(pod.Namespace, pod.Name, "curl", "--max-time", "3", fmt.Sprintf("http://%s:%s", nodeIP, port))
	incomingAddr := string(result)
	incomingAddr = strings.ReplaceAll(incomingAddr, "|", "")
	Expect(checkPodIPs(pod.Status.PodIPs, incomingAddr)).To(BeTrue())

	return nil
}

func checkIngressConnections(egressData egressTemplateData) []error {
	By("get client pod's data")
	pod := &corev1.Pod{}
	Eventually(func() bool {
		err := getResource("default", "pod", egressData.PodName, "", pod)
		if err != nil {
			return false
		}
		return strings.Contains(pod.Name, egressData.PodName)
	}).Should(BeTrue())

	var errs []error
	for _, ip := range pod.Status.PodIPs {
		tmpIP := ip.IP
		addr := net.ParseIP(tmpIP)
		Expect(addr).ToNot(BeNil())
		isv6 := !(addr.To4() != nil)

		if isv6 {
			tmpIP = fmt.Sprintf("[%s]", tmpIP)
		}

		if (!isv6 && enableIPv4Tests) || (isv6 && enableIPv6Tests) {
			By("test connection to client pod on address " + tmpIP)
			if _, err := runOnNode("coil-control-plane", "curl", "--max-time", "3", fmt.Sprintf("http://%s:%s", tmpIP, egressData.Port)); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

func prepareEgressData(port, postfix string) egressTemplateData {
	podName := "nat-client"
	egressName := "egress"
	if postfix != "" {
		podName = fmt.Sprintf("%s-%s", podName, postfix)
		egressName = fmt.Sprintf("%s-%s", egressName, postfix)
	}
	egressData := egressTemplateData{
		PodName:    podName,
		EgressName: egressName,
		Port:       port,
	}

	By(fmt.Sprintf("preparing addresses for %q", egressData.EgressName))
	testIf := os.Getenv(networkInterfaceKey)
	if testIf == "" {
		testIf = "br-"
	}

	if enableIPv4Tests {
		_, ipv4net, err := getLocalIP(testIf, netlink.FAMILY_V4)
		Expect(err).ToNot(HaveOccurred())
		egressData.IPv4Addr = ipv4net.String()
	}

	if enableIPv6Tests {
		_, ipv6net, err := getLocalIP(testIf, netlink.FAMILY_V6)
		Expect(err).ToNot(HaveOccurred())
		egressData.IPv6Addr = ipv6net.String()
	}

	return egressData
}

func deployEgress(egressData egressTemplateData, curDir, tmpDir string) {
	By(fmt.Sprintf("preparing %q manifest", egressData.EgressName))
	egressTemplate := filepath.Join(curDir, "manifests", "egress.yaml.tmpl")
	egressTemplateExec, err := template.New("egress.yaml.tmpl").ParseFiles(egressTemplate)
	Expect(err).ToNot(HaveOccurred())

	egressFilepath := filepath.Join(tmpDir, fmt.Sprintf("%s.yaml", egressData.EgressName))
	egressFile, err := os.Create(egressFilepath)
	Expect(err).ToNot(HaveOccurred())
	defer egressFile.Close()

	err = egressTemplateExec.Execute(egressFile, egressData)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("creating egress %q", egressData.EgressName))
	kubectlSafe(nil, "apply", "-f", egressFilepath)

	By(fmt.Sprintf("checking if %q is ready", egressData.EgressName))
	Eventually(func() int {
		depl := &appsv1.Deployment{}
		err := getResource("internet", "deployments", egressData.EgressName, "", depl)
		if err != nil {
			return 0
		}
		return int(depl.Status.ReadyReplicas)
	}).Should(Equal(1))
}

func deployClient(egressData egressTemplateData, curDir, tmpDir string) *corev1.Pod {
	podTemplate := filepath.Join(curDir, "manifests", "nat-client.yaml.tmpl")
	podTemplateExec, err := template.New("nat-client.yaml.tmpl").ParseFiles(podTemplate)
	Expect(err).ToNot(HaveOccurred())

	if _, err := os.Stat(tmpDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.Mkdir(filepath.Join(curDir, "tmp"), 0o744)
		}
		Expect(err).ToNot(HaveOccurred())
	}

	podFilepath := filepath.Join(tmpDir, fmt.Sprintf("%s.yaml", egressData.PodName))
	podFile, err := os.Create(podFilepath)
	Expect(err).ToNot(HaveOccurred())
	defer podFile.Close()

	err = podTemplateExec.Execute(podFile, egressData)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("creating pod %q", egressData.PodName))
	kubectlSafe(nil, "apply", "-f", podFilepath)

	By(fmt.Sprintf("waiting for %q pod start", egressData.PodName))
	Eventually(func() bool {
		pod := &corev1.Pod{}
		err := getResource("default", "pod", egressData.PodName, "", pod)
		if err != nil {
			return false
		}
		return pod.Status.Phase == corev1.PodRunning
	}).Should(BeTrue())

	By(fmt.Sprintf("get %q pod's data", egressData.PodName))
	pods := &corev1.PodList{}
	Eventually(func() bool {
		err := getResource("internet", "pods", "", fmt.Sprintf("app.kubernetes.io/instance=%s", egressData.EgressName), pods)
		if err != nil {
			return false
		}
		if len(pods.Items) < 1 {
			return false
		}
		return strings.Contains(pods.Items[0].Name, egressData.EgressName)
	}).Should(BeTrue())

	return &pods.Items[0]
}
