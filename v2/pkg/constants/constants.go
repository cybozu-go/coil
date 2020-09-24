package constants

// annotation keys
const (
	AnnPool = "coil.cybozu.com/pool"
)

// Label keys
const (
	LabelPool = "coil.cybozu.com/pool"
	LabelNode = "coil.cybozu.com/node"

	LabelAppName      = "app.kubernetes.io/name"
	LabelAppInstance  = "app.kubernetes.io/instance"
	LabelAppComponent = "app.kubernetes.io/component"
)

// Index keys
const (
	IndexController = ".metadata.controller"
)

// Finalizers
const (
	FinCoil = "coil.cybozu.com"
)

// Keys in CNI_ARGS
const (
	PodNameKey      = "K8S_POD_NAME"
	PodNamespaceKey = "K8S_POD_NAMESPACE"
	PodContainerKey = "K8S_POD_INFRA_CONTAINER_ID"
)

// Environment variables
const (
	EnvNode         = "COIL_NODE_NAME"
	EnvAddresses    = "COIL_POD_ADDRESSES"
	EnvPodNamespace = "COIL_POD_NAMESPACE"
	EnvPodName      = "COIL_POD_NAME"
)

// MetricsNS is the namespace for Prometheus metrics
const MetricsNS = "coil"

// Misc
const (
	DefaultPool = "default"
)
