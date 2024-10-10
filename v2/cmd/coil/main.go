package main

import (
	"context"
	"fmt"
	"time"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	v2 "github.com/cybozu-go/coil/v2"
	"github.com/cybozu-go/coil/v2/pkg/cnirpc"
)

const (
	rpcTimeout      = 1 * time.Minute
	ipamEnableKey   = "ipam"
	egressEnableKey = "egress"
)

func cmdAdd(args *skel.CmdArgs) error {
	conf, err := parseConfig(args.StdinData)
	if err != nil {
		return err
	}

	ipamEnablad, exists := conf.Capabilities[ipamEnableKey]
	if !exists {
		ipamEnablad = true
	}

	if ipamEnablad && conf.PrevResult != nil {
		return types.NewError(types.ErrInvalidNetworkConfig, "coil must be called as the first plugin when IPAM related features are enabled", "")
	}

	cniArgs, err := makeCNIArgs(args, conf)
	if err != nil {
		return err
	}

	conn, err := connect(conf.Socket)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := cnirpc.NewCNIClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	resp, err := client.Add(ctx, cniArgs)
	if err != nil {
		return convertError(err)
	}

	var result types.Result
	if conf.PrevResult != nil {
		result, err = current.NewResultFromResult(conf.PrevResult)
	} else {
		result, err = current.NewResult(resp.Result)
	}

	if err != nil {
		return types.NewError(types.ErrDecodingFailure, "failed to unmarshal result", err.Error())
	}

	return types.PrintResult(result, conf.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	conf, err := parseConfig(args.StdinData)
	if err != nil {
		return err
	}

	cniArgs, err := makeCNIArgs(args, conf)
	if err != nil {
		return err
	}

	conn, err := connect(conf.Socket)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := cnirpc.NewCNIClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	if _, err = client.Del(ctx, cniArgs); err != nil {
		return convertError(err)
	}

	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	conf, err := parseConfig(args.StdinData)
	if err != nil {
		return err
	}

	cniArgs, err := makeCNIArgs(args, conf)
	if err != nil {
		return err
	}

	conn, err := connect(conf.Socket)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := cnirpc.NewCNIClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	if _, err = client.Check(ctx, cniArgs); err != nil {
		return convertError(err)
	}

	return nil
}

func main() {
	skel.PluginMainFuncs(skel.CNIFuncs{Add: cmdAdd, Del: cmdDel, Check: cmdCheck, GC: nil, Status: nil}, version.PluginSupports("0.3.1", "0.4.0", "1.0.0"), fmt.Sprintf("coil %s", v2.Version()))
}
