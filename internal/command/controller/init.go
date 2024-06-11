package controller

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller"
	"github.com/cirruslabs/orchard/internal/netconstants"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
	"math/big"
	"os"
	"time"
)

var ErrInitFailed = errors.New("controller initialization failed")

var controllerCertPath string
var controllerKeyPath string
var sshHostKeyPath string

func FindControllerCertificate(dataDir *controller.DataDir) (tls.Certificate, error) {
	// Prefer user-specified certificate and key
	if controllerCertPath != "" || controllerKeyPath != "" {
		if err := checkBothCertAndKeyAreSpecified(); err != nil {
			return tls.Certificate{}, err
		}

		return tls.LoadX509KeyPair(controllerCertPath, controllerKeyPath)
	}

	// Fall back to loading the certificate from the Orchard data directory
	controllerCert, err := dataDir.ControllerCertificate()
	if err == nil {
		return controllerCert, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return tls.Certificate{}, err
	}

	// Fall back to generating a new certificate
	controllerCert, err = GenerateSelfSignedControllerCertificate()
	if err != nil {
		return controllerCert, err
	}
	if err = dataDir.SetControllerCertificate(controllerCert); err != nil {
		return controllerCert, err
	}

	return controllerCert, nil
}

func FindSSHHostKey(dataDir *controller.DataDir) (ssh.Signer, error) {
	// Prefer user-specified host key
	if sshHostKeyPath != "" {
		hostKeyBytes, err := os.ReadFile(sshHostKeyPath)
		if err != nil {
			return nil, err
		}

		signer, err := ssh.ParsePrivateKey(hostKeyBytes)
		if err != nil {
			return nil, err
		}

		return signer, nil
	}

	// Fall back to loading the host key from the Orchard data directory
	signer, err := dataDir.SSHHostKey()
	if err == nil {
		return signer, err
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Fall back to generating a new host key
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}

	if err := dataDir.SetSSHHostKey(privateKey); err != nil {
		return nil, err
	}

	signer, err = ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, err
	}

	return signer, nil
}

func checkBothCertAndKeyAreSpecified() error {
	if controllerCertPath == "" {
		return fmt.Errorf("%w: when --controller-key is specified, --controller-cert must be specified too",
			ErrInitFailed)
	}

	if controllerKeyPath == "" {
		return fmt.Errorf("%w: when --controller-cert is specified, --controller-key must be specified too",
			ErrInitFailed)
	}

	return nil
}

func GenerateSelfSignedControllerCertificate() (tls.Certificate, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	now := time.Now()

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(0),
		Subject: pkix.Name{
			CommonName: "Orchard Controller",
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{netconstants.DefaultControllerServerName},
	}

	certBytes, err := x509.CreateCertificate(cryptorand.Reader, cert, cert, privateKey.Public(), privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	privateKeyPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return tls.X509KeyPair(certPEMBytes, privateKeyPEMBytes)
}
