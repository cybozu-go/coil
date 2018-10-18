package mtest

import (
	"encoding/json"
	"errors"
	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("pod deployment", func() {
	BeforeEach(initializeCoilData)
	AfterEach(cleanCoilData)

	It("should assign PodIP using address pool", func() {
		addressPool := "10.0.1.0/24"

		By("creating address pool")
		_, _, err := coilctl("pool create default " + addressPool + " 2")
		Expect(err).NotTo(HaveOccurred())

		_, _, err = coilctl("pool show --json default " + addressPool)
		Expect(err).NotTo(HaveOccurred())

		By("deployment Pods")
		_, _, err = kubectl("run nginx --replicas=2 --image=nginx")
		Expect(err).NotTo(HaveOccurred())

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
		stdout, _, err := kubectl("get pods --selector=run=nginx -o json")
		Expect(err).NotTo(HaveOccurred())
		podList := new(corev1.PodList)
		err = json.Unmarshal(stdout, podList)
		Expect(err).NotTo(HaveOccurred())

		_, subnet, err := net.ParseCIDR(addressPool)
		Expect(err).NotTo(HaveOccurred())

		for _, pod := range podList.Items {
			By("checking PodIP is in address pool")
			ip := net.ParseIP(pod.Status.PodIP)
			Expect(subnet.Contains(ip)).To(BeTrue())

			By("checking veth for Pod exists in node")
			stdout, _, err := execAt(pod.Status.HostIP, "ip -d route show "+ip.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).NotTo(BeEmpty())

			By("checking PodIP is advertised as routing table by coild")
			stdout, _, err = execAt(pod.Status.HostIP, "ip route show table 119 "+ip.String()+"/30")
			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).NotTo(BeEmpty())
		}
	})

	It("should access pod in different node", func() {
		addressPool := "10.0.6.0/24"

		By("creating address pool")
		_, _, err := coilctl("pool create default " + addressPool + " 2")
		Expect(err).NotTo(HaveOccurred())

		_, _, err = coilctl("pool show --json default " + addressPool)
		Expect(err).NotTo(HaveOccurred())

		By("deployment nginx Pod")
		_, _, err = kubectl("run nginx --image=nginx --overrides='{\"apiVersion\": \"v1\", \"spec\": { \"nodeSelector\": {\"kubernetes.io/hostname\": \"" + node1 + "\" } } }' --restart=Never")
		Expect(err).NotTo(HaveOccurred())

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
		_, _, err = kubectl("run -it ubuntu-debug --image=quay.io/cybozu/ubuntu-debug:18.04 --overrides='{ \"apiVersion\": \"v1\", \"spec\": { \"nodeSelector\": { \"kubernetes.io/hostname\": \"" + node2 + "\" } } }' --restart=Never --command -- curl http://" + nginxPodIP)
		Expect(err).NotTo(HaveOccurred())
	})
})
