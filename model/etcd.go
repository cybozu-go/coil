package model

import (
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"testing"

	"github.com/coreos/etcd/clientv3"
	"github.com/cybozu-go/etcdutil"
)

// etcdModel implements Model on etcd.
type etcdModel struct {
	etcd *clientv3.Client
}

// NewEtcdModel returns a Model that works on etcd.
func NewEtcdModel(etcd *clientv3.Client) Model {
	return etcdModel{etcd}
}

const (
	etcdClientURL = "http://localhost:12379"
	etcdPeerURL   = "http://localhost:12380"
)

// TestEtcdRun starts etcd for testing
func TestEtcdRun(m *testing.M) int {
	circleci := os.Getenv("CIRCLECI") == "true"
	if circleci {
		code := m.Run()
		os.Exit(code)
	}

	etcdPath, err := ioutil.TempDir("", "coil-test")
	if err != nil {
		log.Fatal(err)
	}

	cmd := exec.Command("etcd",
		"--data-dir", etcdPath,
		"--initial-cluster", "default="+etcdPeerURL,
		"--listen-peer-urls", etcdPeerURL,
		"--initial-advertise-peer-urls", etcdPeerURL,
		"--listen-client-urls", etcdClientURL,
		"--advertise-client-urls", etcdClientURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
		os.RemoveAll(etcdPath)
	}()

	return m.Run()
}

// NewTestEtcdClient returns a etcd client for testing
func NewTestEtcdClient(prefix string) (*clientv3.Client, error) {
	var clientURL string
	circleci := os.Getenv("CIRCLECI") == "true"
	if circleci {
		clientURL = "http://localhost:2379"
	} else {
		clientURL = etcdClientURL
	}

	cfg := etcdutil.NewConfig(prefix)
	cfg.Endpoints = []string{clientURL}
	return etcdutil.NewClient(cfg)
}

// NewTestEtcdModel return a Model that works on etcd for testing.
func NewTestEtcdModel(t *testing.T) etcdModel {
	etcd, err := NewTestEtcdClient(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	return etcdModel{etcd}
}
