package mtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/cybozu-go/coil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// TestCoilController tests coil-controller
func TestCoilController() {
	BeforeEach(func() {
		initializeCoil()
		coilctlSafe("pool", "create", "default", "10.1.0.0/24", "2")

		// Dump current nodes
		kubectl("get", "nodes/"+node1, "-o=json", ">/tmp/node1.json")
		kubectl("get", "nodes/"+node2, "-o=json", ">/tmp/node2.json")
	})
	AfterEach(func() {
		cleanCoil()

		kubectl("apply", "-f", "/tmp/node1.json")
		kubectl("apply", "-f", "/tmp/node2.json")
	})

	It("deletes node's block by watching kubernetes api", func() {
		By("creating pods")
		overrides := fmt.Sprintf(`{ "apiVersion": "v1", "spec": { "nodeSelector": { "kubernetes.io/hostname": "%s" } } }`, node1)
		overrideFile := remoteTempFile(overrides)
		_, stderr, err := kubectl("run", "--generator=run-pod/v1", "nginx", "--image=nginx", "--overrides=\"$(cat "+overrideFile+")\"", "--restart=Never")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		Eventually(func() error {
			stdout, stderr, err := kubectl("get", "pods", "--selector=app.kubernetes.io/name=nginx", "-o=json")
			if err != nil {
				return fmt.Errorf("%v: stderr=%s", err, stderr)
			}

			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}

			if len(podList.Items) > 0 && podList.Items[0].Status.Phase != corev1.PodRunning {
				return errors.New("Pod is not created")
			}
			return nil
		}).Should(Succeed())

		By("deleting node resources")
		_, stderr, err = kubectl("delete", "-f", "/tmp/node1.json")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("checking block is released")
		Eventually(func() error {
			stdout, stderr, err := coilctl("pool", "show", "default", "10.1.0.0/16", "--json")
			if err != nil {
				return fmt.Errorf("%v: stderr=%s", err, stderr)
			}

			ba := new(coil.BlockAssignment)
			err = json.Unmarshal(stdout, ba)
			if err != nil {
				return err
			}

			if _, ok := ba.Nodes[node1]; ok {
				return errors.New("node1 is still in block assignments")
			}
			return nil
		})
	})

	It("deletes node's block on initialization of the controller", func() {
		By("creating pods")
		overrides := fmt.Sprintf(`{ "apiVersion": "v1", "spec": { "nodeSelector": { "kubernetes.io/hostname": "%s" } } }`, node1)
		overrideFile := remoteTempFile(overrides)
		_, stderr, err := kubectl("run", "--generator=run-pod/v1", "nginx", "--image=nginx", "--overrides=\"$(cat "+overrideFile+")\"", "--restart=Never")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		Eventually(func() error {
			stdout, stderr, err := kubectl("get", "pods", "--selector=app.kubernetes.io/name=nginx", "-o=json")
			if err != nil {
				return fmt.Errorf("%v: stderr=%s", err, stderr)
			}

			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}

			if len(podList.Items) > 0 && podList.Items[0].Status.Phase != corev1.PodRunning {
				return errors.New("Pod is not created")
			}
			return nil
		}).Should(Succeed())

		By("deleting coil-controllers Deployment")
		_, stderr, err = kubectl("delete", "deployment/coil-controllers", "--namespace=kube-system")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		Eventually(func() error {
			stdout, stderr, err := kubectl("get", "deployment", "--selector=app.kubernetes.io/name=coil-controllers", "--namespace=kube-system", "-o=json")
			if err != nil {
				return fmt.Errorf("%v: stderr=%s", err, stderr)
			}

			deployments := new(appsv1.DeploymentList)
			err = json.Unmarshal(stdout, deployments)
			if err != nil {
				return err
			}

			if len(deployments.Items) > 0 {
				return errors.New("deployments still exist")
			}
			return nil
		}).Should(Succeed())

		By("deleting Node resources")
		_, stderr, err = kubectl("delete", "-f", "/tmp/node1.json")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		Eventually(func() error {
			stdout, stderr, err := kubectl("get", "nodes", "--selector=kubernetes.io/hostname="+node1, "-o=json")
			if err != nil {
				return fmt.Errorf("%v: stderr=%s", err, stderr)
			}

			nodes := new(corev1.NodeList)
			err = json.Unmarshal(stdout, nodes)
			if err != nil {
				return err
			}

			if len(nodes.Items) > 0 {
				return errors.New("nodes still exist")
			}
			return nil
		}).Should(Succeed())

		deploy, err := ioutil.ReadFile(deployYAMLPath)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err = kubectlWithInput(deploy, "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		By("checking block is released")
		Eventually(func() error {
			stdout, stderr, err := coilctl("pool", "show", "default", "10.1.0.0/16", "--json")
			if err != nil {
				return fmt.Errorf("%v: stderr=%s", err, stderr)
			}

			ba := new(coil.BlockAssignment)
			err = json.Unmarshal(stdout, ba)
			if err != nil {
				return err
			}

			if _, ok := ba.Nodes[node1]; ok {
				return errors.New("node1 is still in block assignments")
			}
			return nil
		})
	})
}
