package coild

import (
	"net/http"
)

type status struct {
	AddressBlocks map[string][]string `json:"address-blocks"`
	Pods          map[string]string   `json:"pods"`
	Status        int                 `json:"status"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		renderError(r.Context(), w, APIErrBadMethod)
		return
	}

	st := status{
		AddressBlocks: make(map[string][]string),
		Pods:          make(map[string]string),
		Status:        http.StatusOK,
	}

	blocks, err := s.db.GetMyBlocks(r.Context(), s.nodeName)
	if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	for k, v := range blocks {
		bl := make([]string, len(v))
		for i, block := range v {
			bl[i] = block.String()
		}
		st.AddressBlocks[k] = bl
	}

	for _, v := range blocks {
		for _, block := range v {
			podIPs, err := s.db.GetAllocatedIPs(r.Context(), block)
			if err != nil {
				renderError(r.Context(), w, InternalServerError(err))
				return
			}
			for k2, v2 := range podIPs {
				st.Pods[k2] = v2.String()
			}
		}
	}

	renderJSON(w, st, http.StatusOK)
}
