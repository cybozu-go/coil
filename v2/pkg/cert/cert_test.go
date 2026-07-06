package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func writeCertFiles(t *testing.T, dir string, serial int64, mtime time.Time) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "coil-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tls.key"), keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, "tls.crt"), mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func serialOf(t *testing.T, cert *tls.Certificate) int64 {
	t.Helper()
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf.SerialNumber.Int64()
}

func TestReloaderLoadsAfterFilesAppear(t *testing.T) {
	dir := t.TempDir()
	r := NewReloader(dir, logr.Discard())

	if _, err := r.GetCertificate(nil); err == nil {
		t.Fatal("expected an error before certificate files exist")
	}

	writeCertFiles(t, dir, 1, time.Now())
	cert, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if serialOf(t, cert) != 1 {
		t.Errorf("unexpected serial: %d", serialOf(t, cert))
	}
}

func TestReloaderReloadsUpdatedFile(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	writeCertFiles(t, dir, 1, base)
	r := NewReloader(dir, logr.Discard())

	if _, err := r.GetCertificate(nil); err != nil {
		t.Fatal(err)
	}

	writeCertFiles(t, dir, 2, base.Add(time.Second))
	cert, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if serialOf(t, cert) != 2 {
		t.Errorf("expected reloaded certificate with serial 2, got %d", serialOf(t, cert))
	}
}

// kubelet updates a Secret volume by atomically swapping the "..data"
// symlink that tls.crt and tls.key point into.
func TestReloaderSymlinkSwap(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()

	makeData := func(name string, serial int64, mtime time.Time) {
		dataDir := filepath.Join(dir, name)
		if err := os.Mkdir(dataDir, 0755); err != nil {
			t.Fatal(err)
		}
		writeCertFiles(t, dataDir, serial, mtime)
	}
	swapData := func(name string) {
		tmp := filepath.Join(dir, "..data_tmp")
		if err := os.Symlink(name, tmp); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, filepath.Join(dir, "..data")); err != nil {
			t.Fatal(err)
		}
	}

	makeData("..data_1", 1, base)
	swapData("..data_1")
	for _, f := range []string{"tls.crt", "tls.key"} {
		if err := os.Symlink(filepath.Join("..data", f), filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}

	r := NewReloader(dir, logr.Discard())
	cert, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if serialOf(t, cert) != 1 {
		t.Errorf("unexpected serial: %d", serialOf(t, cert))
	}

	makeData("..data_2", 2, base.Add(time.Second))
	swapData("..data_2")

	cert, err = r.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if serialOf(t, cert) != 2 {
		t.Errorf("expected certificate from the swapped directory, got serial %d", serialOf(t, cert))
	}
}

func TestReloaderKeepsCacheOnBrokenUpdate(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	writeCertFiles(t, dir, 1, base)
	r := NewReloader(dir, logr.Discard())

	if _, err := r.GetCertificate(nil); err != nil {
		t.Fatal(err)
	}

	certPath := filepath.Join(dir, "tls.crt")
	if err := os.WriteFile(certPath, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	mtime := base.Add(time.Second)
	if err := os.Chtimes(certPath, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	cert, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatalf("broken update should not invalidate the cached certificate: %v", err)
	}
	if serialOf(t, cert) != 1 {
		t.Errorf("expected the cached certificate, got serial %d", serialOf(t, cert))
	}

	if err := os.Remove(certPath); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetCertificate(nil); err != nil {
		t.Fatalf("file removal should not invalidate the cached certificate: %v", err)
	}
}

func TestReloaderTLSHandshake(t *testing.T) {
	dir := t.TempDir()
	r := NewReloader(dir, logr.Discard())

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{GetCertificate: r.GetCertificate})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				_ = conn.(*tls.Conn).Handshake()
				conn.Close()
			}(conn)
		}
	}()

	dial := func() (int64, error) {
		conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return 0, err
		}
		defer conn.Close()
		return conn.ConnectionState().PeerCertificates[0].SerialNumber.Int64(), nil
	}

	base := time.Now()
	writeCertFiles(t, dir, 1, base)
	if _, err := dial(); err != nil {
		t.Fatal(err)
	}

	writeCertFiles(t, dir, 2, base.Add(time.Second))
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			serial, err := dial()
			if err != nil {
				t.Error(err)
				return
			}
			if serial != 2 {
				t.Errorf("expected rotated certificate, got serial %d", serial)
			}
		})
	}
	wg.Wait()
}
