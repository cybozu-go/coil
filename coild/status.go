package coild

import (
	"net/http"
)

type status struct {
	AddressBlocks []string            `json:"address-blocks"`
	Containers    map[string][]string `json:"containers"`
	Status        int                 `json:"status"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		renderError(r.Context(), w, APIErrBadMethod)
		return
	}

	st := status{
		Containers: make(map[string][]string),
		Status:     http.StatusOK,
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ab := range s.addressBlocks {
		st.AddressBlocks = append(st.AddressBlocks, ab.String())
	}
	for k, v := range s.containerIPs {
		ips := make([]string, len(v))
		for i, a := range v {
			ips[i] = a.String()
		}
		st.Containers[k] = ips
	}

	renderJSON(w, st, http.StatusOK)
}
