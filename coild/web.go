package coild

import (
	"net/http"
	"strings"
)

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/status" {
		s.handleStatus(w, r)
		return
	}

	if r.URL.Path == "/ip" {
		s.handleNewIP(w, r)
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/ip/") {
		renderError(r.Context(), w, APIErrNotFound)
		return
	}

	containerID := r.URL.Path[4:]
	if len(containerID) == 0 {
		renderError(r.Context(), w, APIErrBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleIPGet(w, r, containerID)
	case http.MethodDelete:
		s.handleIPDelete(w, r, containerID)
	default:
		renderError(r.Context(), w, APIErrBadMethod)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
}
