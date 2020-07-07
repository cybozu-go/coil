package coild

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/cybozu-go/coil/v1"
	"github.com/cybozu-go/coil/v1/model"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
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

func (s *Server) getAllocatedIP(ctx context.Context, containerID, podNSName string) (net.IP, error) {
	blocks, err := s.db.GetMyBlocks(ctx, s.nodeName)
	if err != nil {
		return nil, err
	}

	for _, v := range blocks {
		for _, b := range v {
			ips, err := s.db.GetAllocatedIPs(ctx, b)
			if err != nil {
				return nil, err
			}

			ip, ok := ips[containerID]
			if ok {
				return ip, nil
			}

			// In version 1.0.2 and before, <namespace>/<pod name> is used as key of ips.  It is up to caller to match such entries.
			if len(podNSName) != 0 {
				ip, ok := ips[podNSName]
				if ok {
					return ip, nil
				}
			}
		}
	}

	return nil, model.ErrNotFound
}

func (s *Server) handleNewIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		renderError(r.Context(), w, APIErrBadMethod)
		return
	}

	input := struct {
		PodNS       string `json:"pod-namespace"`
		PodName     string `json:"pod-name"`
		ContainerID string `json:"container-id"`
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
	if len(input.ContainerID) == 0 {
		renderError(r.Context(), w, BadRequest("no container-id"))
		return
	}

	poolName, err := s.determinePoolName(r.Context(), input.PodNS)
	if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	containerID := input.ContainerID
	_, err = s.getAllocatedIP(r.Context(), containerID, "")
	if err == nil {
		renderError(r.Context(), w, APIErrConflict)
		return
	} else if err != model.ErrNotFound {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	blocks, err := s.db.GetMyBlocks(r.Context(), s.nodeName)
	if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}
	bl := blocks[poolName]

	assignment := coil.IPAssignment{
		ContainerID: containerID,
		Namespace:   input.PodNS,
		Pod:         input.PodName,
		CreatedAt:   time.Now().UTC(),
	}
RETRY:
	fields := well.FieldsFromContext(r.Context())
	for _, block := range bl {
		ip, err := s.db.AllocateIP(r.Context(), block, assignment)
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
		renderJSON(w, resp, http.StatusOK)

		fields["namespace"] = input.PodNS
		fields["pod"] = input.PodName
		fields["containerid"] = containerID
		fields["pool"] = poolName
		fields["ip"] = ip.String()
		log.Info("allocate an address", fields)
		return
	}

	block, err := s.db.AcquireBlock(r.Context(), s.nodeName, poolName)
	fields["pool"] = poolName
	switch err {
	case model.ErrOutOfBlocks:
		fields[log.FnError] = err
		log.Error("no more blocks in pool", fields)
		renderError(r.Context(), w, APIError{
			Status:  http.StatusServiceUnavailable,
			Message: "no more blocks in pool " + poolName,
			Err:     err,
		})
		return
	case model.ErrNotFound:
		fields[log.FnError] = err
		log.Error("address pool is not found", fields)
		renderError(r.Context(), w, APIError{
			Status:  http.StatusInternalServerError,
			Message: "address pool is not found " + poolName,
			Err:     err,
		})
		return
	case nil:
		// nothing to do
	default:
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	fields["block"] = block.String()
	log.Info("acquired new block", fields)

	if !s.dryRun {
		err = addBlockRouting(s.tableID, s.protocolID, block)
		if err != nil {
			fields[log.FnError] = err
			log.Critical("failed to add a block to routing table", fields)
			renderError(r.Context(), w, InternalServerError(err))
			return
		}
	}

	newAddressBlocks := make([]*net.IPNet, len(bl)+1)
	newAddressBlocks[0] = block
	copy(newAddressBlocks[1:], bl)
	bl = newAddressBlocks
	goto RETRY
}

func (s *Server) handleIPGet(w http.ResponseWriter, r *http.Request, containerID string) {
	ip, err := s.getAllocatedIP(r.Context(), containerID, "")
	if err == model.ErrNotFound {
		renderError(r.Context(), w, APIErrNotFound)
		return
	} else if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	resp := addressInfo{
		Address: ip.String(),
		Status:  http.StatusOK,
	}

	renderJSON(w, resp, http.StatusOK)
}

func (s *Server) handleIPDelete(w http.ResponseWriter, r *http.Request, keys []string) {
	containerID := keys[2]
	respNotFoundOK := addressInfo{
		Address: "",
		Status:  http.StatusOK,
	}

	// Handle IPs allocated in version 1.0.2 and before too.
	ip, err := s.getAllocatedIP(r.Context(), containerID, keys[0]+"/"+keys[1])
	if err == model.ErrNotFound {
		renderJSON(w, respNotFoundOK, http.StatusOK)
		return
	} else if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	blocks, err := s.db.GetMyBlocks(r.Context(), s.nodeName)
	if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	var block *net.IPNet
	var poolName string
OUTER:
	for k, bl := range blocks {
		for _, b := range bl {
			if b.Contains(ip) {
				block = b
				poolName = k
				break OUTER
			}
		}
	}

	fields := well.FieldsFromContext(r.Context())
	if block == nil {
		fields["ip"] = ip.String()
		log.Critical("orphaned IP address", fields)
		renderError(r.Context(), w, InternalServerError(errors.New("orphaned IP address")))
		return
	}

	assignment, modRev, err := s.db.GetAddressInfo(r.Context(), ip)
	if err == model.ErrNotFound {
		renderJSON(w, respNotFoundOK, http.StatusOK)
		return
	} else if err != nil {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	// If `ip` was allocated in version 1.0.2 and before, assignment.ContainerID == "".  In such case, free `ip` without checking container ID.
	if assignment.ContainerID != "" && assignment.ContainerID != containerID {
		renderJSON(w, respNotFoundOK, http.StatusOK)
		return
	}

	err = s.db.FreeIP(r.Context(), block, ip, modRev)
	if err != nil && err != model.ErrModRevDiffers {
		renderError(r.Context(), w, InternalServerError(err))
		return
	}

	resp := addressInfo{
		Address: ip.String(),
		Status:  http.StatusOK,
	}

	renderJSON(w, resp, http.StatusOK)

	fields["namespace"] = keys[0]
	fields["pod"] = keys[1]
	fields["containerid"] = keys[2]
	fields["ip"] = ip.String()
	log.Info("free an address", fields)

	// Try to release address block and delete routing table, but this is not critical error even failed.
	ips, err := s.db.GetAllocatedIPs(r.Context(), block)
	if err != nil {
		log.Warn("failed to get allocated IPs", map[string]interface{}{
			log.FnError: err,
			"block":     block.String(),
		})
		return
	}
	if len(ips) > 0 {
		return
	}
	err = s.db.ReleaseBlock(r.Context(), s.nodeName, poolName, block, false)
	if err != nil {
		log.Warn("failed to release address block", map[string]interface{}{
			log.FnError: err,
			"block":     block.String(),
		})
		return
	}
	if !s.dryRun {
		err = deleteBlockRouting(s.tableID, s.protocolID, block)
		if err != nil {
			log.Warn("failed to delete routing table", map[string]interface{}{
				log.FnError: err,
				"block":     block.String(),
			})
		}
	}
	return
}
