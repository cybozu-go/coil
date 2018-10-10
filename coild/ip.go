package coild

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/cybozu-go/coil/model"
)

func (s *Server) determinePoolName(containerID string) (string, error) {
	return "default", nil
}

func (s *Server) handleNewIP(w http.ResponseWriter, r *http.Request) {
	input := struct {
		ContainerID string `json:"container-id"`
		AddressType string `json:"address-type"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		renderError(r.Context(), w, BadRequest(err.Error()))
		return
	}
	if len(input.ContainerID) == 0 {
		renderError(r.Context(), w, BadRequest("no container ID"))
		return
	}

	poolName, err := s.determinePoolName(input.ContainerID)
	if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	bl := s.addressBlocks[poolName]
RETRY:
	for _, block := range bl {
		ip, err := s.db.AllocateIP(r.Context(), block, input.ContainerID)
		if err == model.ErrBlockIsFull {
			continue
		}
		if err != nil {
			renderError(r.Context(), w, InternalServerError(err))
			return
		}

		resp := struct {
			Addresses []string `json:"addresses"`
			Status    int      `json:"status"`
		}{
			Addresses: []string{ip.String()},
			Status:    http.StatusOK,
		}
		s.containerIPs[input.ContainerID] = append(s.containerIPs[input.ContainerID], ip)
		renderJSON(w, resp, http.StatusOK)
		return
	}

	block, err := s.db.AcquireBlock(r.Context(), s.nodeName, poolName)
	if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	newAddressBlocks := make([]*net.IPNet, len(bl)+1)
	newAddressBlocks[0] = block
	copy(newAddressBlocks[1:], bl)
	s.addressBlocks[poolName] = newAddressBlocks
	bl = newAddressBlocks
	goto RETRY
}

func (s *Server) handleIPGet(w http.ResponseWriter, r *http.Request, containerID string) {
}

func (s *Server) handleIPDelete(w http.ResponseWriter, r *http.Request, containerID string) {
}
