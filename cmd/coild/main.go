package main

import (
	"context"
	"flag"
	"net/http"
	"time"

	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/coil/coild"
	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
)

var (
	flagHTTP       = flag.String("http", defaultListenHTTP, "<Listen IP>:<Port number>")
	flagTableID    = flag.Int("table-id", defaultTableID, "Routing table ID to export routes")
	flagProtocolID = flag.Int("protocol-id", defaultProtocolID, "Route author ID")
)

func subMain(ctx context.Context, server *coild.Server) error {
	err := server.Init(ctx)
	if err != nil {
		return nil
	}

	webServer := &well.HTTPServer{
		Server: &http.Server{
			Addr:    *flagHTTP,
			Handler: server,
		},
		ShutdownTimeout: 3 * time.Minute,
	}
	webServer.ListenAndServe()
	return nil
}

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
	server := coild.NewServer(db, *flagTableID, *flagProtocolID)

	well.Go(func(ctx context.Context) error {
		return subMain(ctx, server)
	})
	err = well.Wait()
	if err != nil && !well.IsSignaled(err) {
		log.ErrorExit(err)
	}
}
