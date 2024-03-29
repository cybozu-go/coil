package controllers

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func makePodWithHostNetwork(name string, ips []string, egresses map[string]string) {
	pod := &corev1.Pod{}
	pod.Name = name
	pod.Namespace = "default"
	pod.Annotations = make(map[string]string)
	for k, v := range egresses {
		pod.Annotations["egress.coil.cybozu.com/"+k] = v
	}
	pod.Spec.HostNetwork = true
	var graceSeconds int64
	pod.Spec.TerminationGracePeriodSeconds = &graceSeconds
	pod.Spec.Containers = []corev1.Container{{Name: "c1", Image: "nginx"}}
	err := k8sClient.Create(context.Background(), pod)
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())

	pod.Status.PodIP = ips[0]
	podIPs := make([]corev1.PodIP, len(ips))
	for i, ip := range ips {
		podIPs[i] = corev1.PodIP{IP: ip}
	}
	pod.Status.PodIPs = podIPs
	err = k8sClient.Status().Update(context.Background(), pod)
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
}

func makePod(name string, ips []string, egresses map[string]string, phase corev1.PodPhase) {
	pod := &corev1.Pod{}
	pod.Name = name
	pod.Namespace = "default"
	pod.Annotations = make(map[string]string)
	for k, v := range egresses {
		pod.Annotations["egress.coil.cybozu.com/"+k] = v
	}
	var graceSeconds int64
	pod.Spec.TerminationGracePeriodSeconds = &graceSeconds
	pod.Spec.Containers = []corev1.Container{{Name: "c1", Image: "nginx"}}
	pod.Spec.NodeName = "coil-worker"
	err := k8sClient.Create(context.Background(), pod)
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())

	pod.Status.PodIP = ips[0]
	podIPs := make([]corev1.PodIP, len(ips))
	for i, ip := range ips {
		podIPs[i] = corev1.PodIP{IP: ip}
	}
	pod.Status.PodIPs = podIPs
	pod.Status.Phase = phase
	err = k8sClient.Status().Update(context.Background(), pod)
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
}

func checkMetrics(clientPodCount int) error {
	expected := bytes.NewBufferString(fmt.Sprintf(`
# HELP coil_egress_client_pod_count the number of client pods which use this egress
# TYPE coil_egress_client_pod_count gauge
coil_egress_client_pod_count{egress="egress2",namespace="internet"} %d
`, clientPodCount))
	return testutil.GatherAndCompare(metrics.Registry, expected, "coil_egress_client_pod_count")
}

var _ = Describe("Pod watcher", func() {
	ctx := context.Background()
	var cancel context.CancelFunc
	var ft *mockFoUTunnel
	var eg *mockEgress

	BeforeEach(func() {
		makePod("pod1", []string{"10.1.1.1", "fd01::1"}, nil, corev1.PodRunning)
		makePod("pod2", []string{"10.1.1.2", "fd01::2"}, map[string]string{
			"internet": "egress2",
			"external": "egress1,egress2",
		}, corev1.PodRunning)
		makePod("pod3", []string{"fd01::3"}, map[string]string{
			"internet": "egress1,egress2",
		}, corev1.PodRunning)
		makePod("pod4", []string{"fd01::4"}, map[string]string{
			"external": "egress1",
		}, corev1.PodRunning)

		ctx, cancel = context.WithCancel(context.TODO())
		ft = &mockFoUTunnel{peers: make(map[string]bool)}
		eg = &mockEgress{ips: make(map[string]bool)}
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ToNot(HaveOccurred())

		err = SetupPodWatcher(mgr, "internet", "egress2", ft, true, eg)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			err := mgr.Start(ctx)
			if err != nil {
				panic(err)
			}
		}()
		time.Sleep(100 * time.Millisecond)
	})

	AfterEach(func() {
		cancel()
		err := k8sClient.DeleteAllOf(context.Background(), &corev1.Pod{}, client.InNamespace("default"))
		Expect(err).ShouldNot(HaveOccurred())
		time.Sleep(10 * time.Millisecond)
	})

	It("should handle pre-existing Pods", func() {
		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
			})
		}).Should(BeTrue())

		Eventually(func() bool {
			return reflect.DeepEqual(eg.GetClients(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
			})
		}).Should(BeTrue())

		Expect(checkMetrics(2)).ShouldNot(HaveOccurred())
	})

	It("should handle new Pods", func() {
		makePod("pod5", []string{"10.1.1.5"}, nil, corev1.PodRunning)
		makePod("pod6", []string{"10.1.1.6"}, map[string]string{
			"internet": "egress2",
		}, corev1.PodRunning)
		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
				"10.1.1.6": true,
			})
		}).Should(BeTrue())

		Eventually(func() bool {
			return reflect.DeepEqual(eg.GetClients(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
				"10.1.1.6": true,
			})
		}).Should(BeTrue())

		Expect(checkMetrics(3)).ShouldNot(HaveOccurred())
	})

	It("should check Pod replacement", func() {
		pod1 := &corev1.Pod{}
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "pod1"}, pod1)
		Expect(err).NotTo(HaveOccurred())
		pod1.Annotations = map[string]string{"egress.coil.cybozu.com/internet": "egress2"}
		err = k8sClient.Update(ctx, pod1)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.1": true,
				"fd01::1":  true,
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
			})
		}).Should(BeTrue())

		Eventually(func() bool {
			return reflect.DeepEqual(eg.GetClients(), map[string]bool{
				"10.1.1.1": true,
				"fd01::1":  true,
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
			})
		}).Should(BeTrue())

		Expect(checkMetrics(3)).ShouldNot(HaveOccurred())

		pod3 := &corev1.Pod{}
		err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "pod3"}, pod3)
		Expect(err).NotTo(HaveOccurred())
		pod3.Status.PodIP = "10.1.1.7"
		pod3.Status.PodIPs = []corev1.PodIP{{IP: "10.1.1.7"}, {IP: "fd01::7"}}
		err = k8sClient.Status().Update(ctx, pod3)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.1": true,
				"fd01::1":  true,
				"10.1.1.2": true,
				"fd01::2":  true,
				"10.1.1.7": true,
				"fd01::7":  true,
			})
		}).Should(BeTrue())

		Eventually(func() bool {
			return reflect.DeepEqual(eg.GetClients(), map[string]bool{
				"10.1.1.1": true,
				"fd01::1":  true,
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true, // founat.Egress does not have remove API
				"10.1.1.7": true,
				"fd01::7":  true,
			})
		}).Should(BeTrue())

		Expect(checkMetrics(3)).ShouldNot(HaveOccurred())
	})

	It("should check Pod deletion", func() {
		// Ensure the pod watcher calls AddPeer and AddClient
		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
			}) && reflect.DeepEqual(eg.GetClients(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
			})
		}).Should(BeTrue())

		pod2 := &corev1.Pod{}
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "pod2"}, pod2)
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.Delete(ctx, pod2)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"fd01::3": true,
			})
		}).Should(BeTrue())

		Eventually(func() bool {
			return reflect.DeepEqual(eg.GetClients(), map[string]bool{
				"10.1.1.2": true, // cannot be removed
				"fd01::2":  true, // ditto
				"fd01::3":  true,
			})
		}).Should(BeTrue())

		Expect(checkMetrics(1)).ShouldNot(HaveOccurred())
	})

	It("should ignore pods running in the host network", func() {
		makePodWithHostNetwork("pod6", []string{"10.1.1.6"}, map[string]string{
			"internet": "egress2",
		})
		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
			})
		}).Should(BeTrue())
		Consistently(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
			})
		}, 3).Should(BeTrue())

		Expect(checkMetrics(2)).ShouldNot(HaveOccurred())
	})

	It("should not delete a peer that another pod is reusing", func() {
		makePod("job", []string{"10.1.1.5"}, map[string]string{
			"internet": "egress2",
		}, corev1.PodRunning)

		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
				"10.1.1.5": true,
			})
		}).Should(BeTrue())

		jobPod := &corev1.Pod{}
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "job"}, jobPod)
		Expect(err).NotTo(HaveOccurred())
		jobPod.Status.Phase = corev1.PodSucceeded
		err = k8sClient.Status().Update(context.Background(), jobPod)
		Expect(err).NotTo(HaveOccurred())

		// coil-egress deletes the peer when the job pod completed
		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
			})
		}).Should(BeTrue())

		// another pod reuses the same ip
		makePod("another", []string{"10.1.1.5"}, map[string]string{
			"internet": "egress2",
		}, corev1.PodRunning)

		jobPod = &corev1.Pod{}
		err = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "job"}, jobPod)
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.Delete(ctx, jobPod)
		Expect(err).NotTo(HaveOccurred())

		// coil-egress doesn't delete the peer that the job pod used because another pod is reusing it
		Consistently(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
				"10.1.1.5": true,
			})
		}, 5*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("should not delete a peer when another pod accidentally hits the same one", func() {
		makePod("job", []string{"10.1.1.5"}, map[string]string{
			"internet": "egress2",
		}, corev1.PodRunning)

		Eventually(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
				"10.1.1.5": true,
			})
		}).Should(BeTrue())

		// another pod hits the same ip and trigger the adding pod event
		makePod("another", []string{"10.1.1.5"}, map[string]string{
			"internet": "egress2",
		}, corev1.PodRunning)

		jobPod := &corev1.Pod{}
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "job"}, jobPod)
		Expect(err).NotTo(HaveOccurred())
		jobPod.Status.Phase = corev1.PodSucceeded
		// Trigger the terminating pod event
		err = k8sClient.Status().Update(context.Background(), jobPod)
		Expect(err).NotTo(HaveOccurred())
		// Trigger the deleting pod event
		err = k8sClient.Delete(ctx, jobPod)
		Expect(err).NotTo(HaveOccurred())

		// coil-egress doesn't delete the peer of another pod
		Consistently(func() bool {
			return reflect.DeepEqual(ft.GetPeers(), map[string]bool{
				"10.1.1.2": true,
				"fd01::2":  true,
				"fd01::3":  true,
				"10.1.1.5": true,
			})
		}, 5*time.Second, 1*time.Second).Should(BeTrue())
	})
})
