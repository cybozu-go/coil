package coild

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/log"
	"k8s.io/apimachinery/pkg/types"
)

type addressInfo struct {
	Address string `json:"address"`
	Status  int    `json:"status"`
}

func (s *Server) determinePoolName(ctx context.Context, podNS string) (string, error) {
	_, err := s.db.GetPool(ctx, podNS)
	switch err {
	case nil:
		return podNS, nil
	case model.ErrNotFound:
		return "default", nil
	default:
		return "", err
	}
}

func (s *Server) handleNewIP(w http.ResponseWriter, r *http.Request) {
	input := struct {
		PodNS       string `json:"pod-namespace"`
		PodName     string `json:"pod-name"`
		AddressType string `json:"address-type"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		renderError(r.Context(), w, BadRequest(err.Error()))
		return
	}
	if len(input.PodNS) == 0 {
		renderError(r.Context(), w, BadRequest("no pod namespace"))
		return
	}
	if len(input.PodName) == 0 {
		renderError(r.Context(), w, BadRequest("no pod name"))
		return
	}

	poolName, err := s.determinePoolName(r.Context(), input.PodNS)
	if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	podNSName := types.NamespacedName{
		Namespace: input.PodNS,
		Name:      input.PodName,
	}.String()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.podIPs[podNSName]; ok {
		renderError(r.Context(), w, APIErrConflict)
		return
	}

	bl := s.addressBlocks[poolName]
RETRY:
	for _, block := range bl {
		ip, err := s.db.AllocateIP(r.Context(), block, podNSName)
		if err == model.ErrBlockIsFull {
			continue
		}
		if err != nil {
			renderError(r.Context(), w, InternalServerError(err))
			return
		}

		resp := addressInfo{
			Address: ip.String(),
			Status:  http.StatusOK,
		}
		s.podIPs[podNSName] = ip
		renderJSON(w, resp, http.StatusOK)
		return
	}

	block, err := s.db.AcquireBlock(r.Context(), s.nodeName, poolName)
	if err == model.ErrOutOfBlocks {
		renderError(r.Context(), w, APIError{
			Status:  http.StatusServiceUnavailable,
			Message: "no more blocks in pool " + poolName,
			Err:     err,
		})
	}
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

func (s *Server) handleIPGet(w http.ResponseWriter, r *http.Request, podKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ip, ok := s.podIPs[podKey]
	if !ok {
		renderError(r.Context(), w, APIErrNotFound)
		return
	}

	resp := addressInfo{
		Address: ip.String(),
		Status:  http.StatusOK,
	}

	renderJSON(w, resp, http.StatusOK)
}

func (s *Server) handleIPDelete(w http.ResponseWriter, r *http.Request, podKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ip, ok := s.podIPs[podKey]
	if !ok {
		renderError(r.Context(), w, APIErrNotFound)
		return
	}

	var block *net.IPNet
OUTER:
	for _, blocks := range s.addressBlocks {
		for _, b := range blocks {
			if b.Contains(ip) {
				block = b
				break OUTER
			}
		}
	}

	if block == nil {
		log.Critical("orphaned IP address", map[string]interface{}{
			"ip": ip.String(),
		})
		renderError(r.Context(), w, InternalServerError(errors.New("orphaned IP address")))
		return
	}

	err := s.db.FreeIP(r.Context(), block, ip)
	if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	delete(s.podIPs, podKey)

	resp := addressInfo{
		Address: ip.String(),
		Status:  http.StatusOK,
	}

	renderJSON(w, resp, http.StatusOK)
}
