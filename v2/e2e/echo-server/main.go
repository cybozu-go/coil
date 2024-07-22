package main

import (
	"flag"
	"io"
	"net"
	"net/http"
)

type echoHandler struct {
	withRemoteAddrReply bool
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
		remote += "|"
		w.Write([]byte(remote))
	}
	w.Write(body)
}

func main() {
	var withRemoteAddress bool
	flag.BoolVar(&withRemoteAddress, "reply-remote", false, "if set, echo-server will reply with remote host address (default: false)")
	flag.Parse()

	s := &http.Server{
		Handler: echoHandler{
			withRemoteAddrReply: withRemoteAddress,
		},
	}
	s.ListenAndServe()
}
