package coild

import (
	"context"
	"net"
	"time"

	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScanLoop scans IP address assignment table and Pods information periodically.
func (s *Server) ScanLoop(ctx context.Context, scanInterval, addressExpiration time.Duration) error {
	ticker := time.NewTicker(scanInterval)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		err := s.scan(ctx, addressExpiration)
		if err != nil {
			return err
		}
	}
}

func (s *Server) scan(ctx context.Context, addressExpiration time.Duration) error {
	blockMap, err := s.db.GetMyBlocks(ctx, s.nodeName)
	if err != nil {
		return err
	}

	assigned := make(map[string]*net.IPNet)
	for _, blocks := range blockMap {
		for _, block := range blocks {
			ips, err := s.db.GetAllocatedIPs(ctx, block)
			if err != nil {
				return err
			}

			for _, ip := range ips {
				assigned[ip.String()] = block
			}
		}
	}

	var podIPs []string
	nsl, err := s.k8s.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, ns := range nsl.Items {
		pl, err := s.k8s.CoreV1().Pods(ns.Name).List(metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, pod := range pl.Items {
			if len(pod.Status.PodIP) != 0 {
				podIPs = append(podIPs, pod.Status.PodIP)
			}
		}
	}

OUTER:
	for assignedIPStr, block := range assigned {
		for _, podIP := range podIPs {
			if podIP == assignedIPStr {
				continue OUTER
			}
		}

		assignedIP := net.ParseIP(assignedIPStr)
		assignment, modRev, err := s.db.GetAddressInfo(ctx, assignedIP)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		if now.Before(assignment.CreatedAt.Add(addressExpiration)) {
			continue
		}

		log.Info("free IP address which is assigned in DB but not used in Pods", map[string]interface{}{
			"address":      assignedIP,
			"container_id": assignment.ContainerID,
			"namespace":    assignment.Namespace,
			"pod":          assignment.Pod,
			"created_at":   assignment.CreatedAt,
		})
		err = s.db.FreeIP(ctx, block, assignedIP, modRev)
		if err == model.ErrModRevDiffers {
			log.Info("IP address was already freed", map[string]interface{}{
				"address":      assignedIP,
				"container_id": assignment.ContainerID,
				"namespace":    assignment.Namespace,
				"pod":          assignment.Pod,
				"created_at":   assignment.CreatedAt,
			})
		} else if err != nil {
			return err
		}
	}

	blockMap, err = s.db.GetMyBlocks(ctx, s.nodeName)
	if err != nil {
		return err
	}
	for poolName, blocks := range blockMap {
		for _, block := range blocks {
			ips, err := s.db.GetAllocatedIPs(ctx, block)
			if err != nil {
				return err
			}
			if len(ips) > 0 {
				continue
			}
			err = s.db.ReleaseBlock(ctx, s.nodeName, poolName, block, false)
			if err != nil {
				return err
			}
			if !s.dryRun {
				err = deleteBlockRouting(s.tableID, s.protocolID, block)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
