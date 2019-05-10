package model

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/netutil"
)

func (m etcdModel) GetAddressInfo(ctx context.Context, ip net.IP) (*coil.IPAssignment, int64, error) {
	blocks, err := m.GetAssignedBlocks(ctx)
	if err != nil {
		return nil, 0, err
	}

	var block *net.IPNet
OUTER:
	for _, bl := range blocks {
		for _, b := range bl {
			if b.Contains(ip) {
				block = b
				break OUTER
			}
		}
	}
	if block == nil {
		return nil, 0, ErrNotFound
	}

	offset := netutil.IP4ToInt(ip) - netutil.IP4ToInt(block.IP)

	resp, err := m.etcd.Get(ctx, ipKey(block, int(offset)))
	if err != nil {
		return nil, 0, err
	}
	if len(resp.Kvs) == 0 {
		return nil, 0, ErrNotFound
	}

	kv := resp.Kvs[0]

	assignment := new(coil.IPAssignment)
	err = json.Unmarshal(kv.Value, assignment)
	if err != nil {
		// In older than version 1.0.2, non-json data is used.  Return assignment with empty container ID.
		ts := strings.Split(string(kv.Value), "/")
		if len(ts) != 2 {
			return nil, 0, errors.New("invalid value for " + string(kv.Key) + " in DB: " + string(kv.Value))
		}
		assignment = &coil.IPAssignment{
			ContainerID: "",
			Namespace:   ts[0],
			Pod:         ts[1],
			CreatedAt:   time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		}
	}
	return assignment, kv.ModRevision, nil
}
