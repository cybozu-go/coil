package controller

import (
	"context"
	"net"
	"time"

	"github.com/cybozu-go/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScanLoop scans IP address assignment table and Pods information periodically.
func (c *Controller) ScanLoop(ctx context.Context, scanInterval, addressExpiration time.Duration) error {
	ticker := time.NewTicker(scanInterval)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		err := c.scan(ctx, addressExpiration)
		if err != nil {
			return err
		}
	}
}

func (c *Controller) scan(ctx context.Context, addressExpiration time.Duration) error {
	blockMap, err := c.model.GetAssignedBlocks(ctx)
	if err != nil {
		return err
	}

	assigned := make(map[string]*net.IPNet)
	for _, blocks := range blockMap {
		for _, block := range blocks {
			ips, err := c.model.GetAllocatedIPs(ctx, block)
			if err != nil {
				return err
			}

			for _, ip := range ips {
				assigned[ip.String()] = block
			}
		}
	}

	var podIPs []string
	nsl, err := c.k8s.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, ns := range nsl.Items {
		pl, err := c.k8s.CoreV1().Pods(ns.Name).List(metav1.ListOptions{})
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
		assignment, _, err := c.model.GetAddressInfo(ctx, assignedIP)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		if now.Before(assignment.CreatedAt.Add(addressExpiration)) {
			continue
		}

		log.Warn("found IP address which is assigned in DB but not used in Pods", map[string]interface{}{
			"address":      assignedIP,
			"block":        block,
			"container_id": assignment.ContainerID,
			"namespace":    assignment.Namespace,
			"pod":          assignment.Pod,
			"created_at":   assignment.CreatedAt,
		})
	}

OUTER2:
	for _, podIP := range podIPs {
		for assignedIPStr, _ := range assigned {
			if assignedIPStr == podIP {
				continue OUTER2
			}
		}

		log.Error("found IP address which is used in Pods but not assigned in DB", map[string]interface{}{
			"address": podIP,
		})
	}

	return nil
}
