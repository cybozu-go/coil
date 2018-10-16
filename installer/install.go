package installer

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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
