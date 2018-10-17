package main

import (
	"context"
	"flag"

	"github.com/cybozu-go/cmd"
	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/coil/controller"
	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
)

func main() {
	cfg := coil.NewEtcdConfig()
	cfg.AddFlags(flag.CommandLine)
	flag.Parse()
	cmd.LogConfig{}.Apply()

	err := coil.ResolveEtcdEndpoints(cfg)
	if err != nil {
		log.ErrorExit(err)
	}

	etcd, err := etcdutil.NewClient(cfg)
	if err != nil {
		log.ErrorExit(err)
	}
	defer etcd.Close()

	db := model.NewEtcdModel(etcd)
	cntl, err := controller.NewController(db)
	if err != nil {
		log.ErrorExit(err)
	}

	cmd.Go(func(ctx context.Context) error {
		rev, err := cntl.Sync(ctx)
		if err != nil {
			return err
		}

		return cntl.Watch(ctx, rev)
	})

	err = cmd.Wait()
	if err != nil && !cmd.IsSignaled(err) {
		log.ErrorExit(err)
	}
}
