package main

import (
	"context"
	"flag"

	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/coil/controller"
	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
)

func main() {
	cfg := coil.NewEtcdConfig()
	cfg.AddFlags(flag.CommandLine)
	flag.Parse()
	err := well.LogConfig{}.Apply()
	if err != nil {
		log.ErrorExit(err)
	}

	err = coil.ResolveEtcdEndpoints(cfg)
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

	well.Go(func(ctx context.Context) error {
		rev, err := cntl.Sync(ctx)
		if err != nil {
			return err
		}

		return cntl.Watch(ctx, rev)
	})

	err = well.Wait()
	if err != nil && !well.IsSignaled(err) {
		log.ErrorExit(err)
	}
}
