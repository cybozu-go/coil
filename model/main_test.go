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

const (
	etcdClientURL = "http://localhost:12379"
	etcdPeerURL   = "http://localhost:12380"
)

func testMain(m *testing.M) int {
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

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func newEtcdClient(prefix string) (*clientv3.Client, error) {
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

func newModel(t *testing.T) etcdModel {
	etcd, err := newEtcdClient(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	return etcdModel{etcd}
}
