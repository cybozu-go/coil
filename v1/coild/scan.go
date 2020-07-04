package coild

import (
	"context"
	"time"

	"github.com/cybozu-go/log"
)

// ScanLoop scans IP address assignment table and Pods information periodically.
func (s *Server) ScanLoop(ctx context.Context, scanInterval time.Duration) error {
	ticker := time.NewTicker(scanInterval)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		err := s.scan(ctx)
		if err != nil {
			return err
		}
	}
}

func (s *Server) scan(ctx context.Context) error {
	blockMap, err := s.db.GetMyBlocks(ctx, s.nodeName)
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

			log.Info("free empty address block", map[string]interface{}{
				"block": block,
				"pool":  poolName,
			})
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
