package main

import (
	"flag"
	"fmt"
	"io"
	log "log/slog"
	"net"
	"net/http"
)

type echoHandler struct {
	withRemoteAddrReply bool
	noSeparator         bool
}

func (h echoHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/octet-stream")

	if h.withRemoteAddrReply {
		remote, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !h.noSeparator {
			remote += "|"
		}

		w.Write([]byte(remote))
	}
	w.Write(body)
}

func main() {
	var withRemoteAddress, noSeparator bool
	var port uint
	flag.BoolVar(&withRemoteAddress, "reply-remote", false, "if set, echo-server will reply with remote host address (default: false)")
	flag.UintVar(&port, "port", 80, "set port server should listen on (default: 80)")
	flag.BoolVar(&noSeparator, "no-separator", false, "reply without separator (default: false, only used with -reply-remote)")
	flag.Parse()

	s := &http.Server{
		Addr: fmt.Sprintf(":%d", port),
		Handler: echoHandler{
			withRemoteAddrReply: withRemoteAddress,
			noSeparator:         noSeparator,
		},
	}
	if err := s.ListenAndServe(); err != nil {
		log.Error("server failed", "err", err)
	}
}
