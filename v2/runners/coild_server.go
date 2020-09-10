package runners

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/cybozu-go/coil/v2/pkg/cnirpc"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/ipam"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	"github.com/golang/protobuf/ptypes/empty"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// NewCoildServer returns an implementation of cnirpc.CNIServer for coild.
func NewCoildServer(l net.Listener, mgr manager.Manager, nodeIPAM ipam.NodeIPAM, podNet nodenet.PodNetwork, logger *zap.Logger) manager.Runnable {
	return &coildServer{
		listener:  l,
		apiReader: mgr.GetAPIReader(),
		client:    mgr.GetClient(),
		nodeIPAM:  nodeIPAM,
		podNet:    podNet,
		logger:    logger,
	}
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

var grpcMetrics = grpc_prometheus.NewServerMetrics()

func init() {
	// register grpc_prometheus with controller-runtime's Registry
	metrics.Registry.MustRegister(grpcMetrics)
}

type coildServer struct {
	cnirpc.UnimplementedCNIServer
	listener  net.Listener
	apiReader client.Reader
	client    client.Client
	nodeIPAM  ipam.NodeIPAM
	podNet    nodenet.PodNetwork
	logger    *zap.Logger
}

var _ manager.LeaderElectionRunnable = &coildServer{}

func (s *coildServer) NeedLeaderElection() bool {
	return false
}

func fieldExtractor(fullMethod string, req interface{}) map[string]interface{} {
	args, ok := req.(*cnirpc.CNIArgs)
	if !ok {
		return nil
	}

	ret := make(map[string]interface{})
	if name, ok := args.Args[constants.PodNameKey]; ok {
		ret["pod.name"] = name
	}
	if namespace, ok := args.Args[constants.PodNamespaceKey]; ok {
		ret["pod.namespace"] = namespace
	}
	ret["netns"] = args.Netns
	ret["ifname"] = args.Ifname
	ret["container_id"] = args.ContainerId
	return ret
}

func (s *coildServer) Start(ch <-chan struct{}) error {
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(
		grpc_middleware.ChainUnaryServer(
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(fieldExtractor)),
			grpcMetrics.UnaryServerInterceptor(),
			grpc_zap.UnaryServerInterceptor(s.logger),
		),
	))
	cnirpc.RegisterCNIServer(grpcServer, s)

	// after all services are registered, initialize metrics.
	grpcMetrics.InitializeMetrics(grpcServer)

	// enable server reflection service
	// see https://github.com/grpc/grpc-go/blob/master/Documentation/server-reflection-tutorial.md
	reflection.Register(grpcServer)

	go func() {
		<-ch
		grpcServer.GracefulStop()
	}()

	return grpcServer.Serve(s.listener)
}

func newError(c codes.Code, cniCode cnirpc.ErrorCode, msg, details string) error {
	st := status.New(c, msg)
	st, err := st.WithDetails(&cnirpc.CNIError{Code: cniCode, Msg: msg, Details: details})
	if err != nil {
		panic(err)
	}

	return st.Err()
}

func newInternalError(err error, msg string) error {
	return newError(codes.Internal, cnirpc.ErrorCode_INTERNAL, msg, err.Error())
}

func (s *coildServer) Add(ctx context.Context, args *cnirpc.CNIArgs) (*cnirpc.AddResponse, error) {
	logger := ctxzap.Extract(ctx)

	podName := args.Args[constants.PodNameKey]
	podNS := args.Args[constants.PodNamespaceKey]
	if podName == "" || podNS == "" {
		logger.Sugar().Errorw("missing pod name/namespace", "args", args.Args)
		return nil, newError(codes.InvalidArgument, cnirpc.ErrorCode_INVALID_ENVIRONMENT_VARIABLES,
			"missing pod name/namespace", fmt.Sprintf("%+v", args.Args))
	}

	// TODO: pod will be used for selective NAT feature
	pod := &corev1.Pod{}
	if err := s.apiReader.Get(ctx, client.ObjectKey{Namespace: podNS, Name: podName}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Sugar().Errorw("pod not found", "name", podName, "namespace", podNS)
			return nil, newError(codes.NotFound, cnirpc.ErrorCode_UNKNOWN_CONTAINER, "pod not found", err.Error())
		}
		logger.Sugar().Errorw("failed to get pod", "name", podName, "namespace", podNS, "error", err)
		return nil, newInternalError(err, "failed to get pod")
	}

	// fetch namespace to decide the pool name
	ns := &corev1.Namespace{}
	if err := s.client.Get(ctx, client.ObjectKey{Name: podNS}, ns); err != nil {
		logger.Sugar().Errorw("failed to get namespace", "name", podNS, "error", err)
		return nil, newInternalError(err, "failed to get namespace")
	}
	poolName := constants.DefaultPool
	if v, ok := ns.Annotations[constants.AnnPool]; ok {
		poolName = v
	}

	ipv4, ipv6, err := s.nodeIPAM.Allocate(ctx, poolName, args.ContainerId, args.Ifname)
	if err != nil {
		logger.Sugar().Errorw("failed to allocate address", "error", err)
		return nil, newInternalError(err, "failed to allocate address")
	}

	result, err := s.podNet.Setup(args.Netns, podName, podNS, &nodenet.PodNetConf{
		ContainerId: args.ContainerId,
		IFace:       args.Ifname,
		IPv4:        ipv4,
		IPv6:        ipv6,
		PoolName:    poolName,
	})
	if err != nil {
		if err := s.nodeIPAM.Free(ctx, args.ContainerId, args.Ifname); err != nil {
			logger.Sugar().Warnw("failed to deallocate address", "error", err)
		}
		logger.Sugar().Errorw("failed to setup pod network", "error", err)
		return nil, newInternalError(err, "failed to setup pod network")
	}

	data, err := json.Marshal(result)
	if err != nil {
		if err := s.podNet.Destroy(args.ContainerId, args.Ifname); err != nil {
			logger.Sugar().Warnw("failed to destroy pod network", "error", err)
		}
		if err := s.nodeIPAM.Free(ctx, args.ContainerId, args.Ifname); err != nil {
			logger.Sugar().Warnw("failed to deallocate address", "error", err)
		}
		logger.Sugar().Errorw("failed to marshal the result", "error", err)
		return nil, newInternalError(err, "failed to marshal the result")
	}
	return &cnirpc.AddResponse{Result: data}, nil
}

func (s *coildServer) Del(ctx context.Context, args *cnirpc.CNIArgs) (*empty.Empty, error) {
	logger := ctxzap.Extract(ctx)

	if err := s.podNet.Destroy(args.ContainerId, args.Ifname); err != nil {
		logger.Sugar().Errorw("failed to destroy pod network", "error", err)
		return nil, newInternalError(err, "failed to destroy pod network")
	}

	if err := s.nodeIPAM.Free(ctx, args.ContainerId, args.Ifname); err != nil {
		logger.Sugar().Errorw("failed to free addresses", "error", err)
		return nil, newInternalError(err, "failed to free addresses")
	}
	return &empty.Empty{}, nil
}

func (s *coildServer) Check(ctx context.Context, args *cnirpc.CNIArgs) (*empty.Empty, error) {
	logger := ctxzap.Extract(ctx)

	if err := s.podNet.Check(args.ContainerId, args.Ifname); err != nil {
		logger.Sugar().Errorw("check failed", "error", err)
		return nil, newInternalError(err, "check failed")
	}
	return &empty.Empty{}, nil
}
