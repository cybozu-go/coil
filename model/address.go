package model

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/coreos/etcd/clientv3"
	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/netutil"
)

func (m etcdModel) GetAddressInfo(ctx context.Context, ip net.IP) (*coil.IPAssignment, error) {
	n := netutil.IP4ToInt(ip)

	resp, err := m.etcd.Get(ctx, keyIP, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	for _, kv := range resp.Kvs {
		ts := strings.Split(string(kv.Key)[len(keyIP):], "/")
		if len(ts) != 2 {
			return nil, errors.New("invalid key in DB: " + string(kv.Key))
		}

		blockIP := net.ParseIP(ts[0])
		offset, err := strconv.Atoi(ts[1])
		if err != nil {
			return nil, err
		}

		if netutil.IP4ToInt(blockIP)+uint32(offset) == n {
			assignment := new(coil.IPAssignment)
			err = json.Unmarshal(kv.Value, assignment)
			if err != nil {
				// In older than version 1.0.2, non-json data is used. ignore it.
				return nil, err
			}
			return assignment, nil
		}
	}

	return nil, ErrNotFound
}
