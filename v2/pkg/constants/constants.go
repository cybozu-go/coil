package constants

import "time"

// annotation keys
const (
	AnnPool         = "coil.cybozu.com/pool"
	AnnEgressPrefix = "egress.coil.cybozu.com/"
)

// Label keys
const (
	LabelPool     = "coil.cybozu.com/pool"
	LabelNode     = "coil.cybozu.com/node"
	LabelRequest  = "coil.cybozu.com/request"
	LabelReserved = "coil.cybozu.com/reserved"

	LabelAppName      = "app.kubernetes.io/name"
	LabelAppInstance  = "app.kubernetes.io/instance"
	LabelAppComponent = "app.kubernetes.io/component"
)

// Index keys
const (
	AddressBlockRequestKey = "address-block.request"
	PodNodeNameKey         = "pod.node-name"
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

// RBAC resource names
const (
	// SAEgress is the name of the ServiceAccount for coil-egress
	SAEgress = "coil-egress"

	// CRBEgress is the name of the ClusterRoleBinding for coil-egress
	CRBEgress = "coil-egress"

	// CRBEgressPSP is the name of the ClusterRoleBinding for coil-egress PSP.
	CRBEgressPSP = "psp-coil-egress"
)

// Environment variables
const (
	EnvNode         = "COIL_NODE_NAME"
	EnvAddresses    = "COIL_POD_ADDRESSES"
	EnvPodNamespace = "COIL_POD_NAMESPACE"
	EnvPodName      = "COIL_POD_NAME"
	EnvEgressName   = "COIL_EGRESS_NAME"
)

// Config flags
const (
	IsChained = "IS_CHAINED"
)

// Default config values
const (
	DefautlMetricsAddr            = ":9384"
	DefautlHealthAddr             = ":9385"
	DefautlPodTableId             = 116
	DefautlPodRulePrio            = 2000
	DefautlExportTableId          = 119
	DefautlProtocolId             = 30
	DefaultCompatCalico           = false
	DefaultEgressPort             = 5555
	DefaultRegisterFromMain       = false
	DefaultEnableIPAM             = true
	DefaultEnableEgress           = true
	DefaultAddressBlockGCInterval = 5 * time.Minute

	DefaultEnableCertRotation         = false
	DefaultEnableRestartOnCertRefresh = false
)

// MetricsNS is the namespace for Prometheus metrics
const MetricsNS = "coil"

// Misc
const (
	DefaultPool = "default"
)
