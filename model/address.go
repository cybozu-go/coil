package model

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/coreos/etcd/clientv3"
	"github.com/cybozu-go/netutil"
)

func (m etcdModel) GetAddressInfo(ctx context.Context, ip net.IP) (string, error) {
	n := netutil.IP4ToInt(ip)

	resp, err := m.etcd.Get(ctx, keyIP, clientv3.WithPrefix())
	if err != nil {
		return "", err
	}

	for _, kv := range resp.Kvs {
		ts := strings.Split(string(kv.Key)[len(keyIP):], "/")
		if len(ts) != 2 {
			return "", errors.New("invalid key in DB: " + string(kv.Key))
		}

		blockIP := net.ParseIP(ts[0])
		offset, err := strconv.Atoi(ts[1])
		if err != nil {
			return "", err
		}

		if netutil.IP4ToInt(blockIP)+uint32(offset) == n {
			return string(kv.Value), nil
		}
	}

	return "", ErrNotFound
}
