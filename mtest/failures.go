package mtest

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// TestPodStartup tests pod startup, especially under failure injection.
func TestPodStartup() {
	BeforeEach(initializeCoil)
	AfterEach(cleanCoil)

	It("should confirm successful creation of CKE-managed pods", func() {
		By("waiting for cluster-dns pods to become ready")
		Eventually(func() error {
			stdout, stderr, err := kubectl("get", "-n", "kube-system", "deployment", "cluster-dns", "-o", "json")
			if err != nil {
				return fmt.Errorf("err=%v, stdout=%s, stderr=%s", err, stdout, stderr)
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return fmt.Errorf("err=%v, stdout=%s", err, stdout)
			}

			if deployment.Status.ReadyReplicas != 2 {
				return errors.New("ReadyReplicas != 2")
			}
			return nil
		}).Should(Succeed())
	})

	It("should starts up pod on half-cleaned node", func() {
		By("making uncleaned veth artificially")
		h := sha1.New()
		h.Write([]byte("default.foo"))
		vethName := "veth" + hex.EncodeToString(h.Sum(nil))[:11]
		execSafeAt(node1, "sudo", "ip", "netns", "add", "foo")
		execSafeAt(node1, "sudo", "ip", "netns", "exec", "foo", "ip", "link", "add", "eth0", "type", "veth", "peer", "name", vethName)
		execSafeAt(node1, "sudo", "ip", "netns", "exec", "foo", "ip", "link", "set", vethName, "netns", "1")

		By("creating pod with specific name on specific node")
		podYAML := `
apiVersion: v1
kind: Pod
metadata:
  name: foo
  namespace: default
spec:
  nodeName: %s
  containers:
  - name: nginx
    image: nginx
`
		stdout, stderr, err := kubectlWithInput([]byte(fmt.Sprintf(podYAML, node1)), "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s", stdout, stderr)

		By("waiting for pod to become ready")
		Eventually(func() error {
			stdout, stderr, err := kubectl("get", "-n", "default", "pod", "foo", "-o", "json")
			if err != nil {
				return fmt.Errorf("err=%v, stdout=%s, stderr=%s", err, stdout, stderr)
			}

			pod := new(corev1.Pod)
			err = json.Unmarshal(stdout, pod)
			if err != nil {
				return fmt.Errorf("err=%v, stdout=%s", err, stdout)
			}

			if pod.Status.Phase != corev1.PodRunning {
				return errors.New("Phase != Running")
			}
			return nil
		}).Should(Succeed())
	})
}
