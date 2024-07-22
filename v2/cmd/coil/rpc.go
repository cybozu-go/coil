package main

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/cybozu-go/coil/v2/pkg/cnirpc"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

// makeCNIArgs creates *CNIArgs.
func makeCNIArgs(args *skel.CmdArgs, conf *PluginConf) (*cnirpc.CNIArgs, error) {
	env := &PluginEnvArgs{}
	if err := types.LoadArgs(args.Args, env); err != nil {
		return nil, types.NewError(types.ErrInvalidEnvironmentVariables, "failed to load CNI_ARGS", err.Error())
	}

	argsData := env.Map()
	argsData[constants.IsChained] = strconv.FormatBool(conf.PrevResult != nil)

	ips := []string{}
	interfaces := map[string]bool{}
	if conf.PrevResult != nil {
		prevResult, err := current.GetResult(conf.PrevResult)
		if err != nil {
			return nil, fmt.Errorf("error getting previous CNI result: %w", err)
		}
		for _, ip := range prevResult.IPs {
			ips = append(ips, ip.Address.IP.String())
		}
		for _, intf := range prevResult.Interfaces {
			interfaces[intf.Name] = intf.Sandbox != ""
		}
	}

	cniArgs := &cnirpc.CNIArgs{
		ContainerId: args.ContainerID,
		Netns:       args.Netns,
		Ifname:      args.IfName,
		Args:        argsData,
		Path:        args.Path,
		StdinData:   args.StdinData,
		Ips:         ips,
		Interfaces:  interfaces,
	}

	return cniArgs, nil
}

// connect connects to coild
func connect(sock string) (*grpc.ClientConn, error) {
	dialer := &net.Dialer{}
	dialFunc := func(ctx context.Context, a string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", a)
	}
	resolver.SetDefaultScheme("passthrough")

	conn, err := grpc.NewClient(sock, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(dialFunc))
	if err != nil {
		return nil, types.NewError(types.ErrTryAgainLater, "failed to connect to "+sock, err.Error())
	}
	return conn, nil
}

// convertError turns err returned from gRPC library into CNI's types.Error
func convertError(err error) error {
	st := status.Convert(err)
	details := st.Details()
	if len(details) != 1 {
		return types.NewError(types.ErrInternal, st.Message(), err.Error())
	}

	cniErr, ok := details[0].(*cnirpc.CNIError)
	if !ok {
		types.NewError(types.ErrInternal, st.Message(), err.Error())
	}

	return types.NewError(uint(cniErr.Code), cniErr.Msg, cniErr.Details)
}
