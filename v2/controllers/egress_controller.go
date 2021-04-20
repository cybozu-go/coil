package controllers

import (
	"context"

	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EgressReconciler reconciles a Egress object
type EgressReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Image  string
	Port   int32
}

// +kubebuilder:rbac:groups=coil.cybozu.com,resources=egresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=coil.cybozu.com,resources=egresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch

// coil-controller needs to have access to Pods to grant egress service accounts the same privilege.
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// Reconcile implements Reconciler interface.
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *EgressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	eg := &coilv2.Egress{}
	if err := r.Get(ctx, req.NamespacedName, eg); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get egress")
		return ctrl.Result{}, err
	}
	if eg.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if err := r.reconcileServiceAccount(ctx, logger, req.Namespace); err != nil {
		logger.Error(err, "failed to reconcile service account")
		return ctrl.Result{}, err
	}

	log1 := logger.WithValues("clusterrolebinding", constants.CRBEgress)
	if err := reconcileCRB(ctx, r.Client, log1, constants.CRBEgress); err != nil {
		log1.Error(err, "failed to reconcile cluster role binding")
		return ctrl.Result{}, err
	}

	log2 := logger.WithValues("clusterrolebinding", constants.CRBEgressPSP)
	if err := reconcileCRB(ctx, r.Client, log2, constants.CRBEgressPSP); err != nil {
		log2.Error(err, "failed to reconcile cluster role binding",
			"ClusterRoleBinding", constants.CRBEgressPSP)
		return ctrl.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, logger, eg); err != nil {
		logger.Error(err, "failed to reconcile deployment")
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, logger, eg); err != nil {
		logger.Error(err, "failed to reconcile service")
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, logger, eg); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *EgressReconciler) reconcileServiceAccount(ctx context.Context, log logr.Logger, ns string) error {
	sa := &corev1.ServiceAccount{}
	err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: constants.SAEgress}, sa)
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		sa.Namespace = ns
		sa.Name = constants.SAEgress
		log.Info("creating service account for egress")
		return r.Create(ctx, sa)
	}
	return err
}

func selectorLabels(name string) map[string]string {
	return map[string]string{
		constants.LabelAppName:      "coil",
		constants.LabelAppInstance:  name,
		constants.LabelAppComponent: "egress",
	}
}

func (r *EgressReconciler) reconcilePodTemplate(eg *coilv2.Egress, depl *appsv1.Deployment) {
	target := &depl.Spec.Template
	target.Labels = make(map[string]string)
	target.Annotations = make(map[string]string)

	desired := eg.Spec.Template
	podSpec := &corev1.PodSpec{}
	if desired != nil {
		podSpec = desired.Spec.DeepCopy()
		for k, v := range desired.Annotations {
			target.Annotations[k] = v
		}
		for k, v := range desired.Labels {
			target.Labels[k] = v
		}
	}
	for k, v := range selectorLabels(eg.Name) {
		target.Labels[k] = v
	}

	podSpec.ServiceAccountName = constants.SAEgress
	podSpec.Volumes = r.addVolumes(podSpec.Volumes)

	var egressContainer *corev1.Container
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name != "egress" {
			continue
		}
		egressContainer = &(podSpec.Containers[i])
	}
	if egressContainer == nil {
		podSpec.Containers = append([]corev1.Container{{}}, podSpec.Containers...)
		egressContainer = &(podSpec.Containers[0])
	}
	egressContainer.Name = "egress"
	if egressContainer.Image == "" {
		egressContainer.Image = r.Image
	}
	if len(egressContainer.Command) == 0 {
		egressContainer.Command = []string{"coil-egress"}
	}
	if len(egressContainer.Args) == 0 {
		egressContainer.Args = []string{"--zap-stacktrace-level=panic"}
	}
	egressContainer.Env = append(egressContainer.Env,
		corev1.EnvVar{
			Name:  constants.EnvPodNamespace,
			Value: eg.Namespace,
		},
		corev1.EnvVar{
			Name:  constants.EnvEgressName,
			Value: eg.Name,
		},
		corev1.EnvVar{
			Name: constants.EnvAddresses,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIPs",
				},
			},
		},
	)
	egressContainer.VolumeMounts = r.addVolumeMounts(egressContainer.VolumeMounts)
	egressContainer.SecurityContext = &corev1.SecurityContext{
		Privileged:             pointer.BoolPtr(true),
		ReadOnlyRootFilesystem: pointer.BoolPtr(true),
		Capabilities:           &corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN"}},
	}
	if egressContainer.Resources.Requests == nil {
		egressContainer.Resources.Requests = make(corev1.ResourceList)
	}
	if _, ok := egressContainer.Resources.Requests[corev1.ResourceCPU]; !ok {
		egressContainer.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("100m")
	}
	if _, ok := egressContainer.Resources.Requests[corev1.ResourceMemory]; !ok {
		egressContainer.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("200Mi")
	}
	egressContainer.Ports = []corev1.ContainerPort{
		{Name: "metrics", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
		{Name: "health", ContainerPort: 8081, Protocol: corev1.ProtocolTCP},
	}
	egressContainer.LivenessProbe = &corev1.Probe{
		Handler: corev1.Handler{HTTPGet: &corev1.HTTPGetAction{
			Path:   "/healthz",
			Port:   intstr.FromString("health"),
			Scheme: corev1.URISchemeHTTP,
		}},
	}
	egressContainer.ReadinessProbe = &corev1.Probe{
		Handler: corev1.Handler{HTTPGet: &corev1.HTTPGetAction{
			Path:   "/readyz",
			Port:   intstr.FromString("health"),
			Scheme: corev1.URISchemeHTTP,
		}},
	}

	podSpec.DeepCopyInto(&target.Spec)
}

func (r *EgressReconciler) addVolumes(vols []corev1.Volume) []corev1.Volume {
	noRun := true
	for _, vol := range vols {
		if vol.Name == "run" {
			noRun = false
			break
		}
	}
	if noRun {
		vols = append(vols, corev1.Volume{
			Name: "run",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	vols = append(vols, corev1.Volume{
		Name: "modules",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/lib/modules",
			},
		},
	})
	return vols
}

func (r *EgressReconciler) addVolumeMounts(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	noRun := true
	for _, m := range mounts {
		if m.Name == "run" {
			noRun = false
			break
		}
	}
	if noRun {
		mounts = append(mounts, corev1.VolumeMount{
			MountPath: "/run",
			Name:      "run",
			ReadOnly:  false,
		})
	}

	mounts = append(mounts, corev1.VolumeMount{
		MountPath: "/lib/modules",
		Name:      "modules",
		ReadOnly:  true,
	})

	return mounts
}

func (r *EgressReconciler) reconcileDeployment(ctx context.Context, log logr.Logger, eg *coilv2.Egress) error {
	depl := &appsv1.Deployment{}
	depl.Namespace = eg.Namespace
	depl.Name = eg.Name
	result, err := ctrl.CreateOrUpdate(ctx, r.Client, depl, func() error {
		if depl.DeletionTimestamp != nil {
			return nil
		}

		if depl.Labels == nil {
			depl.Labels = make(map[string]string)
		}
		labels := selectorLabels(eg.Name)
		for k, v := range labels {
			depl.Labels[k] = v
		}

		// set immutable fields only for a new object
		if depl.CreationTimestamp.IsZero() {
			if err := ctrl.SetControllerReference(eg, depl, r.Scheme); err != nil {
				return err
			}
			depl.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		}

		if depl.Spec.Replicas == nil || *depl.Spec.Replicas != eg.Spec.Replicas {
			replicas := eg.Spec.Replicas
			depl.Spec.Replicas = &replicas
		}

		if eg.Spec.Strategy != nil {
			eg.Spec.Strategy.DeepCopyInto(&depl.Spec.Strategy)
		}
		r.reconcilePodTemplate(eg, depl)

		return nil
	})
	if err != nil {
		return err
	}

	if result != controllerutil.OperationResultNone {
		log.Info(string(result) + " deployment")
	}
	return nil
}

func (r *EgressReconciler) reconcileService(ctx context.Context, log logr.Logger, eg *coilv2.Egress) error {
	svc := &corev1.Service{}
	svc.Namespace = eg.Namespace
	svc.Name = eg.Name
	result, err := ctrl.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if svc.DeletionTimestamp != nil {
			return nil
		}

		if svc.Labels == nil {
			svc.Labels = make(map[string]string)
		}
		labels := selectorLabels(eg.Name)
		for k, v := range labels {
			svc.Labels[k] = v
		}

		// set immutable fields only for a new object
		if svc.CreationTimestamp.IsZero() {
			if err := ctrl.SetControllerReference(eg, svc, r.Scheme); err != nil {
				return err
			}
		}

		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Selector = labels
		svc.Spec.Ports = []corev1.ServicePort{{
			Port:       r.Port,
			TargetPort: intstr.FromInt(int(r.Port)),
			Protocol:   corev1.ProtocolUDP,
		}}
		svc.Spec.SessionAffinity = eg.Spec.SessionAffinity
		if eg.Spec.SessionAffinityConfig != nil {
			sac := &corev1.SessionAffinityConfig{}
			eg.Spec.SessionAffinityConfig.DeepCopyInto(sac)
			svc.Spec.SessionAffinityConfig = sac
		}

		return nil
	})
	if err != nil {
		return err
	}

	if result != controllerutil.OperationResultNone {
		log.Info(string(result) + " service")
	}
	return nil
}

func (r *EgressReconciler) updateStatus(ctx context.Context, log logr.Logger, eg *coilv2.Egress) error {
	depl := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, depl); err != nil {
		return err
	}

	sel, err := metav1.LabelSelectorAsSelector(depl.Spec.Selector)
	if err != nil {
		return err
	}
	selString := sel.String()

	changed := false
	if eg.Status.Selector != selString {
		changed = true
		eg.Status.Selector = selString
	}
	if eg.Status.Replicas != depl.Status.AvailableReplicas {
		changed = true
		eg.Status.Replicas = depl.Status.AvailableReplicas
	}

	if changed {
		if err := r.Status().Update(ctx, eg); err != nil {
			return err
		}
		log.Info("updated status")
	}

	return nil
}

// SetupWithManager registers this with the manager.
func (r *EgressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&coilv2.Egress{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get

// GetImage returns the current pod's container image.
// This is intended to prepare the image name for EgressReconciler.
func GetImage(apiReader client.Reader, key client.ObjectKey) (string, error) {
	ctx := context.Background()

	pod := &corev1.Pod{}
	if err := apiReader.Get(ctx, key, pod); err != nil {
		return "", err
	}

	return pod.Spec.Containers[0].Image, nil
}
