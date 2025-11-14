package runners

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	current "github.com/containernetworking/cni/pkg/types/100"
	coilv2 "github.com/cybozu-go/coil/v2/api/v2"
	"github.com/cybozu-go/coil/v2/pkg/cnirpc"
	"github.com/cybozu-go/coil/v2/pkg/config"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/founat"
	"github.com/cybozu-go/coil/v2/pkg/ipam"
	"github.com/cybozu-go/coil/v2/pkg/nodenet"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// GWNets represents networks for a destination.
type GWNets struct {
	Gateway         net.IP
	Networks        []*net.IPNet
	SportAuto       bool
	OriginatingOnly bool
}

// NATSetup represents a NAT setup function for Pods.
type NATSetup interface {
	Hook([]GWNets, string, *zap.Logger) func(ipv4, ipv6 net.IP) error
}

// NewNATSetup creates a NATSetup using founat package.
// `port` is the UDP port number to accept Foo-over-UDP packets.
func NewNATSetup(port int) NATSetup {
	return natSetup{port: port}
}

type natSetup struct {
	port int
}

func (n natSetup) Hook(l []GWNets, backend string, log *zap.Logger) func(ipv4, ipv6 net.IP) error {
	return func(ipv4, ipv6 net.IP) error {
		ft := founat.NewFoUTunnel(n.port, ipv4, ipv6, func(message string) {
			log.Sugar().Info(message)
		})
		if err := ft.Init(); err != nil {
			return err
		}

		cl := founat.NewNatClient(ipv4, ipv6, nil, backend, func(message string) {
			log.Sugar().Info(message)
		})
		if err := cl.Init(); err != nil {
			return err
		}

		for _, gwn := range l {
			link, err := ft.AddPeer(gwn.Gateway, gwn.SportAuto)
			if errors.Is(err, founat.ErrIPFamilyMismatch) {
				// ignore unsupported IP family link
				log.Sugar().Infow("ignored unsupported gateway", "gw", gwn.Gateway)
				continue
			}
			if err != nil {
				return err
			}
			if err := cl.AddEgress(link, gwn.Networks, gwn.OriginatingOnly); err != nil {
				return err
			}
		}

		return nil
	}
}

// NewCoildServer returns an implementation of cnirpc.CNIServer for coild.
func NewCoildServer(l net.Listener, mgr manager.Manager, nodeIPAM ipam.NodeIPAM, podNet nodenet.PodNetwork, setup NATSetup, cfg *config.Config, logger *zap.Logger,
	aliasFunc func(conf *nodenet.PodNetConf, pod *corev1.Pod, ifName string) error) manager.Runnable {
	return &coildServer{
		listener:  l,
		apiReader: mgr.GetAPIReader(),
		client:    mgr.GetClient(),
		nodeIPAM:  nodeIPAM,
		podNet:    podNet,
		natSetup:  setup,
		logger:    logger,
		cfg:       cfg,
		aliasFunc: aliasFunc,
	}
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get
// +kubebuilder:rbac:groups="",resources=namespaces;services,verbs=get;list;watch
// +kubebuilder:rbac:groups=coil.cybozu.com,resources=egresses,verbs=get;list;watch

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
	natSetup  NATSetup
	logger    *zap.Logger
	cfg       *config.Config
	aliasFunc func(conf *nodenet.PodNetConf, pod *corev1.Pod, ifName string) error
}

var _ manager.LeaderElectionRunnable = &coildServer{}

func (s *coildServer) NeedLeaderElection() bool {
	return false
}

func (s *coildServer) Start(ctx context.Context) error {
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(
		logging.UnaryServerInterceptor(
			InterceptorLogger(s.logger),
			logging.WithFieldsFromContextAndCallMeta(loggingFields),
			logging.WithLogOnEvents(logging.FinishCall)),
		grpcMetrics.UnaryServerInterceptor(),
	))
	cnirpc.RegisterCNIServer(grpcServer, s)

	// after all services are registered, initialize metrics.
	grpcMetrics.InitializeMetrics(grpcServer)

	// enable server reflection service
	// see https://github.com/grpc/grpc-go/blob/master/Documentation/server-reflection-tutorial.md
	reflection.Register(grpcServer)

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	s.logger.Info("start periodic nodeIPAM GC")
	gcTicker := time.NewTicker(s.cfg.AddressBlockGCInterval)
	go func(ctx context.Context) {
		for {
			<-gcTicker.C
			if err := s.nodeIPAM.GC(ctx); err != nil {
				s.logger.Sugar().Error("failed to run GC", "error", err)
			}
		}

	}(ctx)

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
	logger := withCtxFields(ctx, s.logger)

	isChained, err := getSettings(args)
	if err != nil {
		return nil, newInternalError(fmt.Errorf("runtime error"), "failed to get CNI arguments")
	}

	if s.cfg.EnableIPAM && isChained {
		return nil, newInternalError(fmt.Errorf("configuration error"), "coil must be called as the first plugin when IPAM related features are enabled")
	}

	pod, err := s.getPodFromArgs(ctx, args, logger)
	if err != nil {
		return nil, newInternalError(err, "failed to get pod")
	}

	var ipv4, ipv6 net.IP
	var poolName string

	if s.cfg.EnableIPAM {
		ns := &corev1.Namespace{}
		if err := s.client.Get(ctx, client.ObjectKey{Name: pod.Namespace}, ns); err != nil {
			logger.Sugar().Errorw("failed to get namespace", "name", pod.Namespace, "error", err)
			return nil, newInternalError(err, "failed to get namespace")
		}
		poolName = constants.DefaultPool
		if v, ok := ns.Annotations[constants.AnnPool]; ok {
			poolName = v
		}

		ipv4, ipv6, err = s.nodeIPAM.Allocate(ctx, poolName, args.ContainerId, args.Ifname)
		if err != nil {
			logger.Sugar().Errorw("failed to allocate address", "error", err)
			return nil, newInternalError(err, "failed to allocate address")
		}
	} else {
		ipv4, ipv6 = getPodIPs(args.Ips)
	}

	result := &current.Result{
		CNIVersion: current.ImplementedSpecVersion,
	}

	config := &nodenet.PodNetConf{
		ContainerId: args.ContainerId,
		IFace:       args.Ifname,
		IPv4:        ipv4,
		IPv6:        ipv6,
		PoolName:    poolName,
	}

	if s.cfg.EnableIPAM {
		result, err = s.podNet.SetupIPAM(args.Netns, pod.Name, pod.Namespace, config)
		if err != nil {
			if err := s.nodeIPAM.Free(ctx, args.ContainerId, args.Ifname); err != nil {
				logger.Sugar().Warnw("failed to deallocate address", "error", err)
			}
			logger.Sugar().Errorw("failed to setup pod network", "error", err)
			return nil, newInternalError(err, "failed to setup pod network IPAM")
		}
	}

	if s.cfg.EnableEgress {
		if !s.cfg.EnableIPAM {
			if err := s.setCoilInterfaceAlias(args.Interfaces, config, pod); err != nil {
				return nil, fmt.Errorf("failed to set interface alias: %w", err)
			}
		}

		hook, err := s.getHook(ctx, pod)
		if err != nil {
			logger.Sugar().Errorw("failed to setup NAT hook", "error", err)
			return nil, newInternalError(err, "failed to setup NAT hook")
		}

		if hook != nil {
			logger.Sugar().Info("enabling NAT")
			if err := s.podNet.SetupEgress(args.Netns, config, hook); err != nil {
				return nil, newInternalError(err, "failed to setup pod network egress")
			}
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		if s.cfg.EnableIPAM {
			if err := s.podNet.Destroy(args.ContainerId, args.Ifname); err != nil {
				logger.Sugar().Warnw("failed to destroy pod network", "error", err)
			}
			if err := s.nodeIPAM.Free(ctx, args.ContainerId, args.Ifname); err != nil {
				logger.Sugar().Warnw("failed to deallocate address", "error", err)
			}
		}
		logger.Sugar().Errorw("failed to marshal the result", "error", err)
		return nil, newInternalError(err, "failed to marshal the result")
	}
	return &cnirpc.AddResponse{Result: data}, nil
}

func (s *coildServer) setCoilInterfaceAlias(interfaces map[string]bool, conf *nodenet.PodNetConf, pod *corev1.Pod) error {
	ifName := ""
	for name, isSandbox := range interfaces {
		if !isSandbox {
			ifName = name
			break
		}
	}

	if err := s.aliasFunc(conf, pod, ifName); err != nil {
		return fmt.Errorf("failed to add link alias: %w", err)
	}

	return nil
}

func ProcessLinkAlias(conf *nodenet.PodNetConf, pod *corev1.Pod, ifName string) error {
	hLink, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("netlink: failed to look up the host-side veth [%s]: %w", ifName, err)
	}

	// give identifier as an alias of host veth
	if err := netlink.LinkSetAlias(hLink, nodenet.GenAlias(conf, string(pod.UID))); err != nil {
		return fmt.Errorf("netlink: failed to set alias: %w", err)
	}
	return nil
}

func getPodIPs(ips []string) (net.IP, net.IP) {
	var ipv4, ipv6 net.IP
	for _, ip := range ips {
		addr := net.ParseIP(ip)
		if addr != nil {
			if ipv4 == nil && addr.To4() != nil {
				ipv4 = addr
			} else if ipv6 == nil {
				ipv6 = addr
			}
		}
		if ipv4 != nil && ipv6 != nil {
			break
		}
	}
	return ipv4, ipv6
}

func (s *coildServer) Del(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {
	logger := withCtxFields(ctx, s.logger)

	if s.cfg.EnableIPAM {
		if err := s.podNet.Destroy(args.ContainerId, args.Ifname); err != nil {
			logger.Sugar().Errorw("failed to destroy pod network", "error", err)
			return nil, newInternalError(err, "failed to destroy pod network")
		}

		if err := s.nodeIPAM.Free(ctx, args.ContainerId, args.Ifname); err != nil {
			logger.Sugar().Errorw("failed to free addresses", "error", err)
			return nil, newInternalError(err, "failed to free addresses")
		}
	}
	return &emptypb.Empty{}, nil
}

func (s *coildServer) Check(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {
	logger := withCtxFields(ctx, s.logger)

	if s.cfg.EnableIPAM {
		if err := s.podNet.Check(args.ContainerId, args.Ifname); err != nil {
			logger.Sugar().Errorw("check failed", "error", err)
			return nil, newInternalError(err, "check failed")
		}
	} else if s.cfg.EnableEgress {
		pod, err := s.getPodFromArgs(ctx, args, logger)
		if err != nil {
			return nil, newInternalError(err, "unable to get pod")
		}

		if err := s.podNet.Check(string(pod.UID), args.Ifname); err != nil {
			logger.Sugar().Errorw("check failed", "error", err)
			return nil, newInternalError(err, "check failed")
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *coildServer) getPodFromArgs(ctx context.Context, args *cnirpc.CNIArgs, logger *zap.Logger) (*corev1.Pod, error) {
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

	return pod, nil
}

func (s *coildServer) getHook(ctx context.Context, pod *corev1.Pod) (nodenet.SetupHook, error) {
	logger := withCtxFields(ctx, s.logger)

	if pod.Spec.HostNetwork {
		// pods running in the host network cannot use egress NAT.
		// In fact, such a pod won't call CNI, so this is just a safeguard.
		return nil, nil
	}

	var egNames []client.ObjectKey

	for k, v := range pod.Annotations {
		if !strings.HasPrefix(k, constants.AnnEgressPrefix) {
			continue
		}

		ns := k[len(constants.AnnEgressPrefix):]
		for _, name := range strings.Split(v, ",") {
			egNames = append(egNames, client.ObjectKey{Namespace: ns, Name: name})
		}
	}
	if len(egNames) == 0 {
		return nil, nil
	}

	var gwlist []GWNets
	for _, n := range egNames {
		eg := &coilv2.Egress{}
		svc := &corev1.Service{}

		if err := s.client.Get(ctx, n, eg); err != nil {
			return nil, newError(codes.FailedPrecondition, cnirpc.ErrorCode_INTERNAL,
				"failed to get Egress "+n.String(), err.Error())
		}
		if err := s.client.Get(ctx, n, svc); err != nil {
			return nil, newError(codes.FailedPrecondition, cnirpc.ErrorCode_INTERNAL,
				"failed to get Service "+n.String(), err.Error())
		}

		for _, clusterIP := range svc.Spec.ClusterIPs {
			svcIP := net.ParseIP(clusterIP)
			if svcIP == nil {
				return nil, newError(codes.Internal, cnirpc.ErrorCode_INTERNAL,
					"invalid ClusterIP in Service "+n.String(), clusterIP)
			}
			var subnets []*net.IPNet

			for _, sn := range eg.Spec.Destinations {
				_, subnet, err := net.ParseCIDR(sn)
				if err != nil {
					return nil, newInternalError(err, "invalid network in Egress "+n.String())
				}
				if (svcIP.To4() != nil) == (subnet.IP.To4() != nil) {
					subnets = append(subnets, subnet)
				}
			}

			if len(subnets) > 0 {
				gwlist = append(gwlist, GWNets{Gateway: svcIP, Networks: subnets,
					SportAuto: eg.Spec.FouSourcePortAuto, OriginatingOnly: s.cfg.OriginatingOnly})
			}
		}
	}

	if len(gwlist) > 0 {
		logger = logger.With(zap.String("pod_name", pod.Name), zap.String("pod_namespace", pod.Namespace))
		logger.Sugar().Infof("gwlist: %v", gwlist)
		return s.natSetup.Hook(gwlist, s.cfg.Backend, logger), nil
	}
	return nil, nil
}

// ref: https://github.com/grpc-ecosystem/go-grpc-middleware/blob/71d7422112b1d7fadd4b8bf12a6f33ba6d22e98e/interceptors/logging/examples/zap/example_test.go#L17
func InterceptorLogger(l *zap.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		logger := l.WithOptions(zap.AddCallerSkip(1)).With(toZapFields(fields)...)

		switch lvl {
		case logging.LevelDebug:
			logger.Debug(msg)
		case logging.LevelInfo:
			logger.Info(msg)
		case logging.LevelWarn:
			logger.Warn(msg)
		case logging.LevelError:
			logger.Error(msg)
		default:
			logger.Warn(fmt.Sprintf("unknown level %v, msg: %s", lvl, msg))
		}
	})
}

func toZapFields(fs []any) []zap.Field {
	zfs := make([]zap.Field, 0, len(fs)/2)

	for i := 0; i < len(fs); i += 2 {
		key := fs[i]
		value := fs[i+1]

		switch v := value.(type) {
		case string:
			zfs = append(zfs, zap.String(key.(string), v))
		case int:
			zfs = append(zfs, zap.Int(key.(string), v))
		case bool:
			zfs = append(zfs, zap.Bool(key.(string), v))
		default:
			zfs = append(zfs, zap.Any(key.(string), v))
		}
	}
	return zfs
}

func loggingFields(_ context.Context, c interceptors.CallMeta) logging.Fields {
	req := c.ReqOrNil
	if req == nil {
		return nil
	}
	args, ok := req.(*cnirpc.CNIArgs)
	if !ok {
		return nil
	}

	ret := make([]any, 0, 10)
	if name, ok := args.Args[constants.PodNameKey]; ok {
		ret = append(ret, []any{"grpc.request.pod.name", name}...)
	}
	if namespace, ok := args.Args[constants.PodNamespaceKey]; ok {
		ret = append(ret, []any{"grpc.request.pod.namespace", namespace}...)
	}
	ret = append(ret, []any{"grpc.request.netns", args.Netns}...)
	ret = append(ret, []any{"grpc.request.ifname", args.Ifname}...)
	ret = append(ret, []any{"grpc.request.container_id", args.ContainerId}...)
	return ret
}

func withCtxFields(ctx context.Context, l *zap.Logger) *zap.Logger {
	return l.With(toZapFields(logging.ExtractFields(ctx))...)
}

func getSettings(args *cnirpc.CNIArgs) (bool, error) {
	isChained := false
	var err error
	_, exists := args.Args[constants.IsChained]
	if exists {
		if isChained, err = strconv.ParseBool(args.Args[constants.IsChained]); err != nil {
			return false, newInternalError(err, "error parsing CNI chaining bool value ")
		}
	}
	return isChained, nil
}
