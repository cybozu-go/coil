package installer

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/cybozu-go/log"
	corev1 "k8s.io/api/core/v1"
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
	taintKeys := make(map[string]bool)
	for _, key := range strings.Split(bootTaint, ",") {
		if key != "" {
			taintKeys[key] = true
		}
	}

	// Return early if user does not use this function because
	// such a user may not grant privileges required to remove
	// taints.
	if len(taintKeys) == 0 {
		return nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	nodes := clientset.CoreV1().Nodes()
	node, err := nodes.Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	var newTaints []corev1.Taint
	for _, taint := range node.Spec.Taints {
		if taintKeys[taint.Key] {
			log.Info("remove taint", map[string]interface{}{
				"node":  nodeName,
				"taint": taint.Key,
			})
			continue
		}
		newTaints = append(newTaints, taint)
	}

	if len(node.Spec.Taints) == len(newTaints) {
		return nil
	}

	node.Spec.Taints = newTaints
	_, err = nodes.Update(node)
	return err
}
