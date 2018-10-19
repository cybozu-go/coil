package installer

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/cybozu-go/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	v4ForwardKey = "net.ipv4.ip_forward"
	v6ForwardKey = "net.ipv6.conf.all.forwarding"
)

// InstallCniConf installs CNI plugin configuration file.
func InstallCniConf(cniConfName, cniEtcDir, cniNetConf, cniNetConfFile string) error {
	data := []byte(cniNetConf)
	if cniNetConf == "" {
		bData, err := ioutil.ReadFile(cniNetConfFile)
		if err != nil {
			return err
		}
		data = bData
	}

	err := os.MkdirAll(cniEtcDir, 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(cniEtcDir, cniConfName))
	if err != nil {
		return err
	}
	defer f.Close()

	err = f.Chmod(0644)
	if err != nil {
		return err
	}

	_, err = f.Write(data)
	if err != nil {
		return err
	}

	return f.Sync()
}

// InstallCoil installs CNI plugin coil.
func InstallCoil(coilPath, cniBinDir string) error {
	f, err := os.Open(coilPath)
	if err != nil {
		return err
	}
	defer f.Close()

	err = os.MkdirAll(cniBinDir, 0755)
	if err != nil {
		return err
	}

	g, err := ioutil.TempFile(cniBinDir, ".tmp")
	if err != nil {
		return err
	}
	defer func() {
		g.Close()
		os.Remove(g.Name())
	}()

	_, err = io.Copy(g, f)
	if err != nil {
		return err
	}

	err = g.Chmod(0755)
	if err != nil {
		return err
	}

	err = g.Sync()
	if err != nil {
		return err
	}

	return os.Rename(g.Name(), filepath.Join(cniBinDir, "coil"))
}

func setForwarding(name string, flag bool) error {
	val := "1\n"
	if !flag {
		val = "0\n"
	}
	return sysctlSet(name, val)
}

// EnableIPForwarding enables IP forwarding.
func EnableIPForwarding() error {
	err := setForwarding(v4ForwardKey, true)
	if err != nil {
		return err
	}

	return setForwarding(v6ForwardKey, true)
}

// RemoveBootTaintFromNode remove bootstrap taints from the node.
func RemoveBootTaintFromNode(nodeName string, bootTaint string) error {
	taintKeys := strings.Split(bootTaint, ",")

	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	node, err := clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	deleted := 0
	for i := range node.Spec.Taints {
		j := i - deleted
		for _, taintKey := range taintKeys {
			if node.Spec.Taints[j].Key == taintKey {
				log.Info("remove taint", map[string]interface{}{
					"node":  nodeName,
					"taint": node.Spec.Taints[j].Key,
				})
				node.Spec.Taints = append(node.Spec.Taints[:j], node.Spec.Taints[j+1:]...)
				deleted++
				break
			}
		}
	}

	_, err = clientset.CoreV1().Nodes().Update(node)
	return err
}
