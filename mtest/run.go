package mtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cybozu-go/well"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	sshTimeout         = 3 * time.Minute
	defaultDialTimeout = 30 * time.Second
	defaultKeepAlive   = 5 * time.Second

	// DefaultRunTimeout is the timeout value for Agent.Run().
	DefaultRunTimeout = 10 * time.Minute
)

var (
	sshClients = make(map[string]*sshAgent)
	httpClient = &well.HTTPClient{Client: &http.Client{}}

	agentDialer = &net.Dialer{
		Timeout:   defaultDialTimeout,
		KeepAlive: defaultKeepAlive,
	}
)

type sshAgent struct {
	client *ssh.Client
	conn   net.Conn
}

func sshTo(address string, sshKey ssh.Signer, userName string) (*sshAgent, error) {
	conn, err := agentDialer.Dial("tcp", address+":22")
	if err != nil {
		fmt.Printf("failed to dial: %s\n", address)
		return nil, err
	}
	config := &ssh.ClientConfig{
		User: userName,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(sshKey),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	err = conn.SetDeadline(time.Now().Add(defaultDialTimeout))
	if err != nil {
		conn.Close()
		return nil, err
	}
	clientConn, channelCh, reqCh, err := ssh.NewClientConn(conn, "tcp", config)
	if err != nil {
		// conn was already closed in ssh.NewClientConn
		return nil, err
	}
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		clientConn.Close()
		return nil, err
	}
	a := sshAgent{
		client: ssh.NewClient(clientConn, channelCh, reqCh),
		conn:   conn,
	}
	return &a, nil
}

func prepareSSHClients(addresses ...string) error {
	sshKey, err := parsePrivateKey(sshKeyFile)
	if err != nil {
		return err
	}

	ch := time.After(sshTimeout)
	for _, a := range addresses {
	RETRY:
		select {
		case <-ch:
			return errors.New("prepareSSHClients timed out")
		default:
		}
		agent, err := sshTo(a, sshKey, "cybozu")
		if err != nil {
			time.Sleep(time.Second)
			goto RETRY
		}
		sshClients[a] = agent
	}

	return nil
}

func parsePrivateKey(keyPath string) (ssh.Signer, error) {
	f, err := os.Open(keyPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return ssh.ParsePrivateKey(data)
}

func execAt(host string, args ...string) (stdout, stderr []byte, e error) {
	return execAtWithStream(host, nil, args...)
}

// WARNING: `input` can contain secret data.  Never output `input` to console.
func execAtWithInput(host string, input []byte, args ...string) (stdout, stderr []byte, e error) {
	var r io.Reader
	if input != nil {
		r = bytes.NewReader(input)
	}
	return execAtWithStream(host, r, args...)
}

// WARNING: `input` can contain secret data.  Never output `input` to console.
func execAtWithStream(host string, input io.Reader, args ...string) (stdout, stderr []byte, e error) {
	agent := sshClients[host]
	return doExec(agent, input, args...)
}

// WARNING: `input` can contain secret data.  Never output `input` to console.
func doExec(agent *sshAgent, input io.Reader, args ...string) ([]byte, []byte, error) {
	err := agent.conn.SetDeadline(time.Now().Add(DefaultRunTimeout))
	if err != nil {
		return nil, nil, err
	}
	defer agent.conn.SetDeadline(time.Time{})

	sess, err := agent.client.NewSession()
	if err != nil {
		return nil, nil, err
	}
	defer sess.Close()

	if input != nil {
		sess.Stdin = input
	}
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	sess.Stdout = outBuf
	sess.Stderr = errBuf
	err = sess.Run(strings.Join(args, " "))
	return outBuf.Bytes(), errBuf.Bytes(), err
}

func execSafeAt(host string, args ...string) []byte {
	stdout, stderr, err := execAt(host, args...)
	ExpectWithOffset(1, err).To(Succeed(), "[%s] %v: %s", host, args, stderr)
	return stdout
}

func execAtLocal(cmd string, args ...string) ([]byte, error) {
	var stdout bytes.Buffer
	command := exec.Command(cmd, args...)
	command.Stdout = &stdout
	command.Stderr = GinkgoWriter
	err := command.Run()
	if err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func localTempFile(body string) *os.File {
	f, err := ioutil.TempFile("", "cke-mtest")
	Expect(err).NotTo(HaveOccurred())
	_, err = f.WriteString(body)
	Expect(err).NotTo(HaveOccurred())
	err = f.Close()
	Expect(err).NotTo(HaveOccurred())
	return f
}

func remoteTempFile(body string) string {
	f, err := ioutil.TempFile("", "cke-mtest")
	Expect(err).NotTo(HaveOccurred())
	defer f.Close()
	_, err = f.WriteString(body)
	Expect(err).NotTo(HaveOccurred())
	_, err = f.Seek(0, os.SEEK_SET)
	Expect(err).NotTo(HaveOccurred())
	remoteFile := filepath.Join("/tmp", filepath.Base(f.Name()))
	_, _, err = execAtWithStream(host1, f, "dd", "of="+f.Name())
	Expect(err).NotTo(HaveOccurred())
	return remoteFile
}

func coilctl(args ...string) ([]byte, []byte, error) {
	args = append([]string{"/opt/bin/coilctl"}, args...)
	return execAt(host1, args...)
}

func coilctlSafe(args ...string) []byte {
	args = append([]string{"/opt/bin/coilctl"}, args...)
	return execSafeAt(host1, args...)
}

func kubectl(args ...string) ([]byte, []byte, error) {
	args = append([]string{"/opt/bin/kubectl"}, args...)
	return execAt(host1, args...)
}

func kubectlWithInput(input []byte, args ...string) ([]byte, []byte, error) {
	args = append([]string{"/opt/bin/kubectl"}, args...)
	return execAtWithInput(host1, input, args...)
}

func isNodeReady(node corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			if cond.Status == corev1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func checkFileExists(host, file string) error {
	_, _, err := execAt(host, "sudo", "test", "-f", file)
	return err
}

func checkSysctlParam(host, param string) string {
	stdout, _, err := execAt(host, "sysctl", "-n", param)
	Expect(err).ShouldNot(HaveOccurred())
	return string(stdout)
}

func etcdctl(args ...string) ([]byte, []byte, error) {
	args = append([]string{"env", "ETCDCTL_API=3", "/opt/bin/etcdctl", "--endpoints=https://" + node1 + ":2379", "--cert=/tmp/coil.crt", "--key=/tmp/coil.key", "--cacert=/tmp/coil-ca.crt"}, args...)
	return execAt(host1, args...)
}

func initializeCoil() {
	deploy, err := ioutil.ReadFile(deployYAMLPath)
	Expect(err).ShouldNot(HaveOccurred())
	_, stderr, err := kubectlWithInput(deploy, "apply", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "stderr: %s", stderr)

	Eventually(func() error {
		stdout, _, err := kubectl("get", "daemonsets/coil-node", "--namespace=kube-system", "-o=json")
		if err != nil {
			return err
		}

		daemonset := new(appsv1.DaemonSet)
		err = json.Unmarshal(stdout, daemonset)
		if err != nil {
			return err
		}

		if daemonset.Status.NumberReady != 2 {
			return errors.New("NumberReady is not 2")
		}
		return nil
	}).Should(Succeed())

	_, stderr, err = kubectl("create", "namespace", "mtest")
	Expect(err).ShouldNot(HaveOccurred(), "stderr: %s", stderr)

	_, stderr, err = kubectl("config", "set-context", "default", "--namespace=mtest")
	Expect(err).ShouldNot(HaveOccurred(), "stderr: %s", stderr)
}

func cleanCoil() {
	_, stderr, err := kubectl("config", "set-context", "default", "--namespace=kube-system")
	Expect(err).ShouldNot(HaveOccurred(), "stderr: %s", stderr)

	_, stderr, err = kubectl("delete", "namespace", "mtest")
	Expect(err).ShouldNot(HaveOccurred(), "stderr: %s", stderr)

	deploy, err := ioutil.ReadFile(deployYAMLPath)
	Expect(err).ShouldNot(HaveOccurred())
	_, stderr, err = kubectlWithInput(deploy, "delete", "-f", "-")
	Expect(err).ShouldNot(HaveOccurred(), "stderr: %s", stderr)

	_, stderr, err = etcdctl("del", "/coil/", "--prefix")
	Expect(err).ShouldNot(HaveOccurred(), "stderr: %s", stderr)
}
