package mtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("pod deployment", func() {
	BeforeEach(initializeCoil)
	AfterEach(cleanCoil)

	It("should assign PodIP using address pool", func() {
		addressPool := "10.0.1.0/24"

		By("creating address pool")
		coilctl("pool create default " + addressPool + " 2")
		coilctl("pool show --json default " + addressPool)

		By("deployment Pods")
		_, stderr, err := kubectl("run nginx --replicas=2 --image=nginx")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("waiting pods are ready")
		Eventually(func() error {
			stdout, _, err := kubectl("get deployments/nginx -o json")
			if err != nil {
				return err
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if deployment.Status.ReadyReplicas != 2 {
				return errors.New("ReadyReplicas is not 2")
			}
			return nil
		}).Should(Succeed())

		By("checking PodIPs are assigned")
		stdout, stderr, err := kubectl("get pods --selector=run=nginx -o json")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		podList := new(corev1.PodList)
		err = json.Unmarshal(stdout, podList)
		Expect(err).NotTo(HaveOccurred())

		_, subnet, err := net.ParseCIDR(addressPool)
		Expect(err).NotTo(HaveOccurred())

		for _, pod := range podList.Items {
			By("checking PodIP is in address pool")
			ip := net.ParseIP(pod.Status.PodIP)
			Expect(subnet.Contains(ip)).To(BeTrue(), "subnet: %s, ip: %s", subnet, ip)

			By("checking veth for Pod exists in node")
			stdout, _, err := execAt(pod.Status.HostIP, "ip -d route show "+ip.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).NotTo(BeEmpty())

			By("checking PodIP is advertised as routing table by coild")
			stdout, _, err = execAt(pod.Status.HostIP, "ip route show table 119 "+ip.String()+"/30")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).NotTo(BeEmpty())
		}

		By("checking PodIPs are released")
		_, stderr, err = kubectl("delete deployments/nginx")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("waiting pod is deleted")
		Eventually(func() error {
			stdout, stderr, err := kubectl("get pods --selector=run=nginx -o json")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			Expect(err).NotTo(HaveOccurred())

			if len(podList.Items) > 0 {
				return errors.New("pods --selector=run=nginx are remaining")
			}
			return nil
		}).Should(Succeed())

		Expect(err).NotTo(HaveOccurred())
		for _, pod := range podList.Items {
			By("checking veth for Pod exists in node")
			ip := net.ParseIP(pod.Status.PodIP)
			stdout, _, err := execAt(pod.Status.HostIP, "ip -d route show "+ip.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(BeEmpty())
		}

		By("checking PodIPs are reused")
		_, stderr, err = kubectl("run nginx --replicas=2 --image=nginx")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Eventually(func() error {
			stdout, _, err := kubectl("get deployments/nginx -o json")
			if err != nil {
				return err
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if deployment.Status.ReadyReplicas != 2 {
				return errors.New("ReadyReplicas is not 2")
			}
			return nil
		}).Should(Succeed())

		By("checking PodIPs are assigned")
		stdout, stderr, err = kubectl("get pods --selector=run=nginx -o json")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		podList2 := new(corev1.PodList)
		err = json.Unmarshal(stdout, podList2)
		Expect(err).NotTo(HaveOccurred())

		ips1 := []string{podList.Items[0].Status.PodIP, podList.Items[1].Status.PodIP}
		ips2 := []string{podList2.Items[0].Status.PodIP, podList2.Items[1].Status.PodIP}

		sort.Strings(ips1)
		sort.Strings(ips2)

		Expect(ips1).To(Equal(ips2))
	})

	It("should expose all blocks", func() {
		addressPool := "10.0.6.0/24"

		By("creating address pool")
		coilctl("pool create default " + addressPool + " 1")
		coilctl("pool show --json default " + addressPool)

		By("creating 4 pods to node1")
		overrides := fmt.Sprintf(`{ "apiVersion": "apps/v1beta1", "spec": { "template": { "spec": { "nodeSelector": { "kubernetes.io/hostname": "%s" } } } } }`, node1)
		_, stderr, err := kubectl("run nginx --image=nginx --replicas=4 --overrides='" + overrides + "'")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		Eventually(func() error {
			stdout, stderr, err := kubectl("get pods --selector=run=nginx -o json")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			Expect(err).NotTo(HaveOccurred())

			if len(podList.Items) != 4 {
				return errors.New("Four pods are not created")
			}
			for _, pod := range podList.Items {
				if pod.Status.Phase != corev1.PodRunning {
					return errors.New("All pods are note running")
				}
			}
			return nil
		}).Should(Succeed())

		stdout, _, err := execAt(node1, "ip route show table 119 10.0.6.0/31")
		Expect(err).NotTo(HaveOccurred())
		Expect(stdout).NotTo(BeEmpty())

		stdout, _, err = execAt(node1, "ip route show table 119 10.0.6.2/31")
		Expect(err).NotTo(HaveOccurred())
		Expect(stdout).NotTo(BeEmpty())
	})

	It("should access pod in different node", func() {
		addressPool := "10.0.6.0/24"

		By("creating address pool")
		coilctl("pool create default " + addressPool + " 2")
		coilctl("pool show --json default " + addressPool)

		By("deployment nginx Pod")
		overrides := fmt.Sprintf(`{ "apiVersion": "v1", "spec": { "nodeSelector": { "kubernetes.io/hostname": "%s" } } }`, node1)
		_, stderr, err := kubectl("run nginx --image=nginx --overrides='" + overrides + "' --restart=Never")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("waiting pods are ready")
		Eventually(func() error {
			stdout, _, err := kubectl("get pods/nginx -o json")
			if err != nil {
				return err
			}

			pod := new(corev1.Pod)
			err = json.Unmarshal(stdout, pod)
			if err != nil {
				return err
			}

			if pod.Status.Phase != corev1.PodRunning {
				return errors.New("pod is not Running")
			}
			if len(pod.Status.PodIP) == 0 {
				return errors.New("pod is not assigned IP")
			}
			return nil
		}).Should(Succeed())

		pod := new(corev1.Pod)
		stdout, _, err := kubectl("get pods/nginx -o json")
		Expect(err).NotTo(HaveOccurred())
		err = json.Unmarshal(stdout, pod)
		Expect(err).NotTo(HaveOccurred())
		nginxPodIP := pod.Status.PodIP

		By("executing curl to nginx Pod from ubuntu-debug Pod in different node")
		overrides = fmt.Sprintf(`{ "apiVersion": "v1", "spec": { "nodeSelector": { "kubernetes.io/hostname": "%s" } } }`, node2)
		_, _, err = kubectl("run -it ubuntu-debug --image=quay.io/cybozu/ubuntu-debug:18.04 --overrides='" + overrides + "' --restart=Never --command -- curl http://" + nginxPodIP)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should assign PodIP using address pool whose name matches the specified namespace", func() {
		defaultAddressPool := "10.0.1.0/24"
		addressPool := "10.0.3.0/24"

		By("creating address pool")
		coilctl("pool create default " + defaultAddressPool + " 2")
		coilctl("pool create dmz " + addressPool + " 2")

		By("deployment Pods in dmz namespace")
		_, stderr, err := kubectl("create namespace dmz")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		_, stderr, err = kubectl("run nginx --replicas=2 --image=nginx --namespace=dmz")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("waiting pods are ready")
		Eventually(func() error {
			stdout, _, err := kubectl("get deployments/nginx --namespace=dmz -o json")
			if err != nil {
				return err
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if deployment.Status.ReadyReplicas != 2 {
				return errors.New("ReadyReplicas is not 2")
			}
			return nil
		}).Should(Succeed())

		By("checking PodIPs are assigned")
		stdout, stderr, err := kubectl("get pods --selector=run=nginx --namespace=dmz -o json")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		podList := new(corev1.PodList)
		err = json.Unmarshal(stdout, podList)
		Expect(err).NotTo(HaveOccurred())

		_, subnet, err := net.ParseCIDR(addressPool)
		Expect(err).NotTo(HaveOccurred())

		for _, pod := range podList.Items {
			By("checking PodIP is in address pool")
			ip := net.ParseIP(pod.Status.PodIP)
			Expect(subnet.Contains(ip)).To(BeTrue(), "subnet: %s, ip: %s", subnet, ip)
		}
	})
})
