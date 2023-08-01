package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/expfmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var testIPv6 = os.Getenv("TEST_IPV6") == "true"

var _ = Describe("Coil", func() {
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

	It("should export metrics", func() {
		By("checking port 9384 for coild")
		out, err := runOnNode("coil-worker", "curl", "-sf", "http://localhost:9384/metrics")
		Expect(err).ShouldNot(HaveOccurred())

		mfs, err := (&expfmt.TextParser{}).TextToMetricFamilies(bytes.NewReader(out))
		Expect(err).NotTo(HaveOccurred())
		Expect(mfs).NotTo(BeEmpty())

		By("checking port 9388 for coil-router")
		out, err = runOnNode("coil-worker", "curl", "-sf", "http://localhost:9388/metrics")
		Expect(err).ShouldNot(HaveOccurred())

		mfs, err = (&expfmt.TextParser{}).TextToMetricFamilies(bytes.NewReader(out))
		Expect(err).NotTo(HaveOccurred())
		Expect(mfs).NotTo(BeEmpty())
	})

	// This series of tests confirms the following things:
	// - coil can call coild gRPC method appropriately
	// - coild runs gRPC server
	// - coil-controller can process BlockRequest
	// - coil-router setups the kernel routing table
	It("should allow pods on different nodes to communicate", func() {
		By("creating the default pool")
		manifest := "manifests/default_pool.yaml"
		if testIPv6 {
			manifest = "manifests/default_pool_v6.yaml"
		}
		kubectlSafe(nil, "apply", "-f", manifest)

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
		if testIPv6 {
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
		if testIPv6 {
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

	It("should be able to run Egress pods", func() {
		By("creating internet namespace")
		kubectlSafe(nil, "create", "ns", "internet")

		By("defining Egress in the internet namespace")
		kubectlSafe(nil, "apply", "-f", "manifests/egress.yaml")

		By("checking pod deployments")
		Eventually(func() int {
			depl := &appsv1.Deployment{}
			err := getResource("internet", "deployments", "egress", "", depl)
			if err != nil {
				return 0
			}
			return int(depl.Status.ReadyReplicas)
		}).Should(Equal(2))
	})

	It("should be able to run NAT client pods", func() {
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
	})

	It("should allow NAT traffic over foo-over-udp tunnel", func() {
		var fakeIP, fakeURL string
		if testIPv6 {
			fakeIP = "2606:4700:4700::9999"
			fakeURL = fmt.Sprintf("http://[%s]", fakeIP)
		} else {
			fakeIP = "9.9.9.9"
			fakeURL = "http://" + fakeIP
		}

		By("setting a fake global address to coil-control-plane")
		_, err := runOnNode("coil-control-plane", "ip", "link", "add", "dummy-fake", "type", "dummy")
		Expect(err).NotTo(HaveOccurred())
		_, err = runOnNode("coil-control-plane", "ip", "link", "set", "dummy-fake", "up")
		Expect(err).NotTo(HaveOccurred())
		if testIPv6 {
			_, err = runOnNode("coil-control-plane", "ip", "address", "add", fakeIP+"/128", "dev", "dummy-fake", "nodad")
		} else {
			_, err = runOnNode("coil-control-plane", "ip", "address", "add", fakeIP+"/32", "dev", "dummy-fake")
		}
		Expect(err).NotTo(HaveOccurred())

		By("running HTTP server on coil-control-plane")
		go runOnNode("coil-control-plane", "/usr/local/bin/echotest")
		time.Sleep(100 * time.Millisecond)

		By("debug 1")
		stdout := execSafe(nil, "sudo", "ip", "netns", "list-id")
		fmt.Println(string(stdout))

		By("debug 2")
		stdout = execSafe(nil, "docker", "inspect", "--format={{.State.Pid}}", "coil-control-plane")
		fmt.Println(string(stdout))

		By("debug 3")
		stdout = execSafe(nil, "sudo", "ip", "netns", "attach", "coil-control-plane", strings.TrimSpace(string(stdout)))
		fmt.Println(string(stdout))

		By("debug 4")
		stdout = execSafe(nil, "sudo", "ip", "netns", "list")
		fmt.Println(string(stdout))

		By("debug 5")
		stdout = execSafe(nil, "sudo", "ip", "netns", "exec", "coil-control-plane", "netstat", "-tln")

		By("debug 6")
		stdout = execSafe(stdout, "grep", "-w", "80")
		fmt.Println(string(stdout))

		By("debug 7")
		stdout = execSafe(nil, "sudo", "ip", "netns", "exec", "coil-control-plane", "ip", "a", "show", "dummy-fake")
		fmt.Println(string(stdout))

		By("debug 100")
		cmd := exec.Command(kubectlCmd, "exec", "nat-client", "--", "curl", "-svvv", fakeURL)
		cmd.Stderr = os.Stderr
		_, err = cmd.Output()
		Expect(err).NotTo(HaveOccurred())

		By("sending and receiving HTTP request from nat-client")
		data := make([]byte, 1<<20) // 1 MiB
		resp := kubectlSafe(data, "exec", "-i", "nat-client", "--", "curl", "-sf", "-T", "-", fakeURL)
		Expect(resp).To(HaveLen(1 << 20))

		By("running the same test 100 times")
		for i := 0; i < 100; i++ {
			time.Sleep(1 * time.Millisecond)
			resp := kubectlSafe(data, "exec", "-i", "nat-client", "--", "curl", "-sf", "-T", "-", fakeURL)
			Expect(resp).To(HaveLen(1 << 20))
		}
	})
})
