package controllers

import (
	"context"
	"errors"
	"time"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func makeEgress(name string) *coilv2.Egress {
	eg := &coilv2.Egress{}
	eg.Namespace = "default"
	eg.Name = name
	eg.Spec.Destinations = []string{"10.1.2.0/24", "fd03::/120"}
	eg.Spec.Replicas = 1
	eg.Spec.SessionAffinity = corev1.ServiceAffinityClientIP
	return eg
}

var _ = Describe("Egress reconciler", func() {
	ctx := context.Background()
	var cancel context.CancelFunc

	BeforeEach(func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:         scheme,
			LeaderElection: false,
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
			Controller: config.Controller{
				SkipNameValidation: ptr.To(true),
			},
		})
		Expect(err).ToNot(HaveOccurred())

		egr := &EgressReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Image:  "coil:dev",
			Port:   5555,
		}
		err = egr.SetupWithManager(mgr)
		Expect(err).ToNot(HaveOccurred())

		err = SetupCRBReconciler(mgr)
		Expect(err).ToNot(HaveOccurred())

		ctx, cancel = context.WithCancel(context.TODO())
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
		time.Sleep(10 * time.Millisecond)
	})

	It("should create Deployment, Service, and ServiceAccount", func() {
		By("creating an Egress")
		eg := makeEgress("eg1")
		err := k8sClient.Create(ctx, eg)
		Expect(err).ShouldNot(HaveOccurred())

		By("checking Deployment, Service, and ServiceAccount")
		var depl *appsv1.Deployment
		var svc *corev1.Service
		Eventually(func() error {
			depl = &appsv1.Deployment{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, depl)
		}).Should(Succeed())
		Eventually(func() error {
			svc = &corev1.Service{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, svc)
		}).Should(Succeed())
		Eventually(func() error {
			sa := &corev1.ServiceAccount{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: "coil-egress"}, sa)
		}).Should(Succeed())

		// serializer := k8sjson.NewSerializerWithOptions(k8sjson.DefaultMetaFactory, scheme, scheme,
		// 	k8sjson.SerializerOptions{Yaml: true})
		// serializer.Encode(depl, os.Stdout)
		// serializer.Encode(svc, os.Stdout)

		Expect(depl.OwnerReferences).To(HaveLen(1))
		Expect(depl.Spec.Replicas).NotTo(BeNil())
		Expect(*depl.Spec.Replicas).To(Equal(int32(1)))

		Expect(depl.Spec.Template.Labels).To(HaveKeyWithValue(constants.LabelAppName, "coil"))
		Expect(depl.Spec.Template.Labels).To(HaveKeyWithValue(constants.LabelAppComponent, "egress"))
		Expect(depl.Spec.Template.Labels).To(HaveKeyWithValue(constants.LabelAppInstance, eg.Name))
		Expect(depl.Spec.Template.Spec.ServiceAccountName).To(Equal("coil-egress"))
		Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(2))

		var egressContainer *corev1.Container
		for i := range depl.Spec.Template.Spec.Containers {
			c := &depl.Spec.Template.Spec.Containers[i]
			if c.Name == "egress" {
				egressContainer = c
				break
			}
		}
		Expect(egressContainer).NotTo(BeNil())
		Expect(egressContainer.Image).To(Equal("coil:dev"))
		Expect(egressContainer.Command).To(Equal([]string{"coil-egress"}))
		Expect(egressContainer.Env).To(HaveLen(3))
		Expect(egressContainer.VolumeMounts).To(HaveLen(2))
		Expect(egressContainer.SecurityContext).NotTo(BeNil())
		Expect(egressContainer.SecurityContext.ReadOnlyRootFilesystem).NotTo(BeNil())
		Expect(*egressContainer.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
		Expect(egressContainer.Resources.Requests).To(HaveKey(corev1.ResourceCPU))
		Expect(egressContainer.Resources.Requests).To(HaveKey(corev1.ResourceMemory))
		Expect(egressContainer.Ports).To(HaveLen(2))
		Expect(egressContainer.LivenessProbe).NotTo(BeNil())
		Expect(egressContainer.ReadinessProbe).NotTo(BeNil())

		Expect(svc.OwnerReferences).To(HaveLen(1))
		Expect(svc.Spec.Selector).To(HaveKeyWithValue(constants.LabelAppName, "coil"))
		Expect(svc.Spec.Selector).To(HaveKeyWithValue(constants.LabelAppComponent, "egress"))
		Expect(svc.Spec.Selector).To(HaveKeyWithValue(constants.LabelAppInstance, eg.Name))
		Expect(svc.Spec.Ports).Should(HaveLen(1))
		Expect(svc.Spec.Ports[0].Port).Should(Equal(int32(5555)))
		Expect(svc.Spec.Ports[0].Protocol).Should(Equal(corev1.ProtocolUDP))
		Expect(svc.Spec.SessionAffinity).Should(Equal(corev1.ServiceAffinityClientIP))

		By("checking status")
		Eventually(func() error {
			eg := &coilv2.Egress{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "eg1"}, eg)
			if err != nil {
				return err
			}

			if eg.Status.Selector == "" {
				return errors.New("status is not updated")
			}

			return nil
		}).Should(Succeed())
	})

	It("should allow customization of Deployment", func() {
		By("creating an Egress")
		eg := makeEgress("eg2")
		eg.Spec.Strategy = &appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}
		err := k8sClient.Create(ctx, eg)
		Expect(err).ShouldNot(HaveOccurred())

		By("checking Deployment")
		var depl *appsv1.Deployment
		Eventually(func() error {
			depl = &appsv1.Deployment{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, depl)
		}).Should(Succeed())

		Expect(depl.Spec.Strategy.Type).To(Equal(appsv1.RecreateDeploymentStrategyType))

		By("changing Egress")
		Eventually(func() error {
			eg = &coilv2.Egress{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "eg2"}, eg)
			if err != nil {
				return err
			}
			eg.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
			maxSurge := intstr.FromInt(3)
			maxUnavailable := intstr.FromInt(2)
			eg.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
				MaxSurge:       &maxSurge,
				MaxUnavailable: &maxUnavailable,
			}
			eg.Spec.Template = &coilv2.EgressPodTemplate{
				Metadata: coilv2.Metadata{
					Annotations: map[string]string{
						"ann1": "qqq",
					},
					Labels: map[string]string{
						"foo":                  "bar",
						constants.LabelAppName: "hoge", // should be ignored
					},
				},
				Spec: corev1.PodSpec{
					SchedulerName: "hoge-scheduler",
					Containers: []corev1.Container{
						{Name: "sidecar", Image: "nginx"},
					},
					Volumes: []corev1.Volume{
						{Name: "dummy", VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/dummy"},
						}},
					},
				},
			}
			return k8sClient.Update(ctx, eg)
		}).Should(Succeed())

		By("checking deployment after update")
		Eventually(func() error {
			depl = &appsv1.Deployment{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, depl)
			if err != nil {
				return err
			}
			if len(depl.Spec.Template.Spec.Containers) != 2 {
				return errors.New("deployment has not been updated")
			}
			return nil
		}).Should(Succeed())

		Expect(depl.Spec.Strategy.Type).To(Equal(appsv1.RollingUpdateDeploymentStrategyType))
		Expect(depl.Spec.Strategy.RollingUpdate).NotTo(BeNil())
		Expect(depl.Spec.Strategy.RollingUpdate.MaxSurge.IntVal).To(Equal(int32(3)))
		Expect(depl.Spec.Strategy.RollingUpdate.MaxUnavailable.IntVal).To(Equal(int32(2)))
		Expect(depl.Spec.Template.Annotations).To(HaveKeyWithValue("ann1", "qqq"))
		Expect(depl.Spec.Template.Labels).To(HaveKeyWithValue("foo", "bar"))
		Expect(depl.Spec.Template.Labels).To(HaveKeyWithValue(constants.LabelAppName, "coil"))
		Expect(depl.Spec.Template.Spec.SchedulerName).To(Equal("hoge-scheduler"))
		Expect(depl.Spec.Template.Spec.Volumes).To(HaveLen(3))
		var sidecar, egressContainer *corev1.Container
		for i := range depl.Spec.Template.Spec.Containers {
			if depl.Spec.Template.Spec.Containers[i].Name == "egress" {
				egressContainer = &depl.Spec.Template.Spec.Containers[i]
				continue
			}
			if depl.Spec.Template.Spec.Containers[i].Name == "sidecar" {
				sidecar = &depl.Spec.Template.Spec.Containers[i]
			}
		}
		Expect(sidecar).NotTo(BeNil())
		Expect(sidecar.Image).To(Equal("nginx"))
		Expect(egressContainer).NotTo(BeNil())

		By("changing Egress to customize egress container")
		Eventually(func() error {
			eg = &coilv2.Egress{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "eg2"}, eg)
			if err != nil {
				return err
			}
			eg.Spec.Template = &coilv2.EgressPodTemplate{
				Metadata: coilv2.Metadata{
					Annotations: nil,
					Labels:      nil,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "egress",
							Image: "mycoil",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("2"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("2"),
								},
							}},
					},
				},
			}
			return k8sClient.Update(ctx, eg)
		}).Should(Succeed())

		By("checking deployment after update")
		Eventually(func() error {
			depl = &appsv1.Deployment{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, depl)
			if err != nil {
				return err
			}
			if len(depl.Spec.Template.Spec.Containers) != 1 {
				return errors.New("deployment has not been updated")
			}
			return nil
		}).Should(Succeed())

		Expect(depl.Spec.Template.Labels).NotTo(HaveKey("foo"))
		Expect(depl.Spec.Template.Spec.SchedulerName).To(Equal("default-scheduler"))
		Expect(depl.Spec.Template.Spec.Containers).To(HaveLen(1))

		egressContainer = &depl.Spec.Template.Spec.Containers[0]
		Expect(egressContainer.Image).To(Equal("mycoil"))
		Expect(egressContainer.Command).To(Equal([]string{"coil-egress"}))
		Expect(egressContainer.Env).To(HaveLen(3))
		Expect(egressContainer.Resources.Requests).To(HaveKey(corev1.ResourceCPU))
		res := egressContainer.Resources.Requests[corev1.ResourceCPU]
		Expect(res.Equal(resource.MustParse("2"))).To(BeTrue())
		Expect(egressContainer.Resources.Requests).To(HaveKey(corev1.ResourceMemory))
		Expect(egressContainer.Resources.Limits).To(HaveKey(corev1.ResourceCPU))
		Expect(egressContainer.Ports).To(HaveLen(2))
		Expect(egressContainer.LivenessProbe).NotTo(BeNil())
		Expect(egressContainer.ReadinessProbe).NotTo(BeNil())
	})

	It("should allow customization of Service", func() {
		By("creating an Egress")
		var timeout int32 = 100
		eg := makeEgress("eg3")
		eg.Spec.SessionAffinity = corev1.ServiceAffinityNone
		eg.Spec.SessionAffinityConfig = &corev1.SessionAffinityConfig{
			ClientIP: &corev1.ClientIPConfig{
				TimeoutSeconds: &timeout,
			},
		}
		err := k8sClient.Create(ctx, eg)
		Expect(err).ShouldNot(HaveOccurred())

		By("checking Service")
		var svc *corev1.Service
		Eventually(func() error {
			svc = &corev1.Service{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, svc)
		}).Should(Succeed())

		Expect(svc.Spec.Ports).Should(HaveLen(1))
		Expect(svc.Spec.SessionAffinity).To(Equal(corev1.ServiceAffinityNone))

		By("changing Egress to change SessionAffinity")
		Eventually(func() error {
			eg = &coilv2.Egress{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "eg3"}, eg)
			if err != nil {
				return err
			}
			eg.Spec.SessionAffinity = corev1.ServiceAffinityClientIP
			eg.Spec.SessionAffinityConfig = &corev1.SessionAffinityConfig{
				ClientIP: &corev1.ClientIPConfig{
					TimeoutSeconds: &timeout,
				},
			}
			return k8sClient.Update(ctx, eg)
		}).Should(Succeed())

		By("checking service after update")
		Eventually(func() error {
			svc = &corev1.Service{}
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, svc)
			if err != nil {
				return err
			}
			if svc.Spec.SessionAffinity != corev1.ServiceAffinityClientIP {
				return errors.New("service has not been updated")
			}
			return nil
		}).Should(Succeed())

		Expect(svc.Spec.SessionAffinityConfig).NotTo(BeNil())
		cfg := svc.Spec.SessionAffinityConfig
		Expect(cfg.ClientIP).NotTo(BeNil())
		Expect(cfg.ClientIP.TimeoutSeconds).NotTo(BeNil())
		Expect(*cfg.ClientIP.TimeoutSeconds).To(Equal(int32(100)))
	})

	It("should reconcile resources soon", func() {
		By("creating an Egress")
		eg := makeEgress("eg4")
		err := k8sClient.Create(ctx, eg)
		Expect(err).ShouldNot(HaveOccurred())

		By("checking Deployment and Service")
		var depl *appsv1.Deployment
		var svc *corev1.Service
		Eventually(func() error {
			depl = &appsv1.Deployment{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, depl)
		}).Should(Succeed())
		Eventually(func() error {
			svc = &corev1.Service{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, svc)
		}).Should(Succeed())

		By("deleting deployment")
		err = k8sClient.Delete(ctx, depl)
		Expect(err).ShouldNot(HaveOccurred())

		By("checking deployment recreation")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, &appsv1.Deployment{})
		}).Should(Succeed())

		By("deleting service")
		err = k8sClient.Delete(ctx, svc)
		Expect(err).ShouldNot(HaveOccurred())

		By("checking service recreation")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, &corev1.Service{})
		}).Should(Succeed())
	})

	It("should reconcile ClusterRoleBindings", func() {
		By("checking coil-egress ClusterRoleBinding")
		var crb *rbacv1.ClusterRoleBinding
		Eventually(func() int {
			crb = &rbacv1.ClusterRoleBinding{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "coil-egress"}, crb)
			if err != nil {
				return 0
			}
			return len(crb.Subjects)
		}).Should(Equal(1))

		Expect(crb.RoleRef.Name).To(Equal("coil-egress"))
		Expect(crb.RoleRef.Kind).To(Equal("ClusterRole"))

		By("creating another egress on namespace egtest")
		eg := makeEgress("eg5")
		eg.Namespace = "egtest"
		err := k8sClient.Create(ctx, eg)
		Expect(err).ShouldNot(HaveOccurred())

		By("checking coil-egress ClusterRoleBinding again")
		Eventually(func() int {
			crb = &rbacv1.ClusterRoleBinding{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "coil-egress"}, crb)
			if err != nil {
				return 0
			}
			return len(crb.Subjects)
		}).Should(Equal(2))

		saNS := make(map[string]bool)
		for _, s := range crb.Subjects {
			Expect(s.Kind).To(Equal("ServiceAccount"))
			Expect(s.Name).To(Equal("coil-egress"))
			saNS[s.Namespace] = true
		}

		Expect(saNS).To(HaveKey("default"))
		Expect(saNS).To(HaveKey("egtest"))

		By("creating psp-coil-egress ClusterRoleBinding")
		pspCRB := &rbacv1.ClusterRoleBinding{}
		pspCRB.Name = "psp-coil-egress"
		pspCRB.RoleRef = crb.RoleRef
		err = k8sClient.Create(ctx, pspCRB)

		By("checking psp-coil-egress reconciliation result")
		// this is done by crbReconciler
		Eventually(func() int {
			pspCRB = &rbacv1.ClusterRoleBinding{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "psp-coil-egress"}, pspCRB)
			if err != nil {
				return 0
			}
			return len(pspCRB.Subjects)
		}).Should(Equal(2))

		saNS = make(map[string]bool)
		for _, s := range pspCRB.Subjects {
			saNS[s.Namespace] = true
		}

		Expect(saNS).To(HaveKey("default"))
		Expect(saNS).To(HaveKey("egtest"))
	})

	It("shouldn't remove annotations when reconciling deployments", func() {
		By("creating an Egress")
		eg := makeEgress("eg6")
		err := k8sClient.Create(ctx, eg)
		Expect(err).ShouldNot(HaveOccurred())

		By("checking Deployment")
		var depl *appsv1.Deployment
		Eventually(func() error {
			depl = &appsv1.Deployment{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, depl)
		}).Should(Succeed())

		By("updating restartedAt annotation and a label")
		if depl.Spec.Template.Annotations == nil {
			depl.Spec.Template.Annotations = make(map[string]string)
		}
		depl.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = "2022-01-01T00:00:00+00:00"
		depl.Spec.Template.Labels["foo"] = "bar"
		Eventually(func() error {
			return k8sClient.Update(ctx, depl)
		}).Should(Succeed())

		By("checking to take over annotations and deleting labels")
		var updatedDepl *appsv1.Deployment
		Eventually(func() error {
			updatedDepl = &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, updatedDepl); err != nil {
				return err
			}
			_, ok := updatedDepl.Spec.Template.Labels["foo"]
			if ok {
				return errors.New("labels key foo must be deleted")
			}
			_, ok = updatedDepl.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]
			if !ok {
				return errors.New("kubectl.kubernetes.io/restartedAt annotation must be set")
			}
			return nil
		}).Should(Succeed())
	})

	It("should create PDB", func() {
		By("creating an Egress")
		eg := makeEgress("eg7")
		minAvailable := intstr.FromInt(1)
		eg.Spec.PodDisruptionBudget = &coilv2.EgressPDBSpec{
			MinAvailable: &minAvailable,
		}
		err := k8sClient.Create(ctx, eg)
		Expect(err).ShouldNot(HaveOccurred())

		By("checking PodDisruptionBudget")
		var pdb *policyv1.PodDisruptionBudget
		Eventually(func() error {
			pdb = &policyv1.PodDisruptionBudget{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, pdb)
		}).Should(Succeed())

		Expect(*pdb.Spec.MinAvailable).To(Equal(intstr.FromInt(1)))
		Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue(constants.LabelAppName, "coil"))
	})
})
