package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	host         = flag.String("host", "coilv2-webhook-service.kube-system.svc", "TLS hostname")
	validFor     = flag.Duration("duration", 36500*24*time.Hour, "Duration that certificate is valid for")
	outDir       = flag.String("outdir", ".", "Directory where the certificate files are created")
	commonName   = flag.String("cn", "coilv2-webhook-service", "Certificate common name")
	outCert      = flag.String("certname", "cert.pem", "Certificate filename")
	outKey       = flag.String("keyname", "key.pem", "Key filename")
	authority    = flag.String("ca", "", "Certificate authority")
	authorityKey = flag.String("cakey", "", "Certificate authority")
)

func main() {
	flag.Parse()

	var ca *x509.Certificate
	var priv *rsa.PrivateKey
	var err error
	if *authority != "" {
		if ca, priv, err = readCA(filepath.Join(*outDir, *authority), filepath.Join(*outDir, *authorityKey)); err != nil {
			log.Fatal(err)
		}
	} else {
		priv, err = rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			log.Fatal(err)
		}
	}

	isCA := (ca == nil)

	keyUsage := x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	extKeyUsage := []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	if isCA {
		keyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
		extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageClientAuth)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(*validFor)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: *commonName,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              keyUsage,
		ExtKeyUsage:           extKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
		DNSNames:              dnsAliases(*host),
	}

	parent := ca
	if isCA {
		parent = &template
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, parent, priv.Public(), priv)
	if err != nil {
		log.Fatalf("failed to create certificate: %v", err)
	}

	_, err = os.Stat(*outDir)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		err = os.MkdirAll(*outDir, 0755)
		if err != nil {
			log.Fatalf("failed to create output directory: %v", err)
		}
	default:
		log.Fatalf("stat %s failed: %v", *outDir, err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		log.Fatalf("failed to marshal private key: %v", err)
	}

	outputPEM(filepath.Join(*outDir, *outCert), "CERTIFICATE", certBytes)
	outputPEM(filepath.Join(*outDir, *outKey), "PRIVATE KEY", privBytes)
}

func dnsAliases(host string) []string {
	parts := strings.Split(host, ".")
	aliases := make([]string, len(parts))
	for i := 0; i < len(parts); i++ {
		aliases[i] = strings.Join(parts[0:len(parts)-i], ".")
	}
	return aliases
}

func outputPEM(fname string, pemType string, data []byte) {
	f, err := os.OpenFile(fname, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("failed to open %s: %v", fname, err)
	}
	defer f.Close()

	err = pem.Encode(f, &pem.Block{Type: pemType, Bytes: data})
	if err != nil {
		log.Fatalf("failed to encode: %v", err)
	}

	err = f.Sync()
	if err != nil {
		log.Fatalf("failed to fsync: %v", err)
	}
}

func readCA(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	certFile, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading file '%s': %w", certPath, err)
	}

	caData, _ := pem.Decode(certFile)

	ca, err := x509.ParseCertificate(caData.Bytes)
	if err != nil {
		log.Fatal(err)
	}

	keyFile, err := os.ReadFile(keyPath)
	if err != nil {
		log.Fatal(err)
	}

	cakData, _ := pem.Decode(keyFile)
	key, err := x509.ParsePKCS8PrivateKey(cakData.Bytes)
	if err != nil {
		log.Fatal(err)
	}

	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("type assertion error - object is not of type *rsa.PrivateKey")
	}

	return ca, privateKey, nil
}
