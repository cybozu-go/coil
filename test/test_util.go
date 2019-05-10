package test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/coreos/etcd/clientv3"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
)

// TestEtcdRun starts etcd for testing
func TestEtcdRun(m *testing.M, clientPort, peerPort string) int {
	circleci := os.Getenv("CIRCLECI") == "true"
	if circleci {
		code := m.Run()
		os.Exit(code)
	}

	etcdPath, err := ioutil.TempDir("", "coil-test")
	if err != nil {
		log.ErrorExit(err)
	}

	cmd := exec.Command("etcd",
		"--data-dir", etcdPath,
		"--initial-cluster", "default=http://localhost:"+peerPort,
		"--listen-peer-urls", "http://localhost:"+peerPort,
		"--initial-advertise-peer-urls", "http://localhost:"+peerPort,
		"--listen-client-urls", "http://localhost:"+clientPort,
		"--advertise-client-urls", "http://localhost:"+clientPort)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		log.ErrorExit(err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
		os.RemoveAll(etcdPath)
	}()

	return m.Run()
}

// NewTestEtcdClient returns a etcd client for testing
func NewTestEtcdClient(t *testing.T, clientPort string) *clientv3.Client {
	var clientURL string
	circleci := os.Getenv("CIRCLECI") == "true"
	if circleci {
		clientURL = "http://localhost:2379"
	} else {
		clientURL = "http://localhost:" + clientPort
	}

	cfg := etcdutil.NewConfig(t.Name() + "/")
	cfg.Endpoints = []string{clientURL}

	etcd, err := etcdutil.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return etcd
}
