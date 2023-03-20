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
	"math/big"
	"time"
)

var ErrInitFailed = errors.New("controller initialization failed")

var controllerCertPath string
var controllerKeyPath string
var serviceAccountName string
var serviceAccountToken string

func FindControllerCertificate(dataDir *controller.DataDir) (controllerCert tls.Certificate, err error) {
	if controllerCertPath != "" || controllerKeyPath != "" {
		// if external certificate is specified, use it
		if err := checkBothCertAndKeyAreSpecified(); err != nil {
			return controllerCert, err
		}

		controllerCert, err = tls.LoadX509KeyPair(controllerCertPath, controllerCertPath)
		if err != nil {
			return controllerCert, err
		}
	} else if !dataDir.ControllerCertificateExists() {
		// otherwise, generate a self-signed certificate if it's not already present
		controllerCert, err = GenerateSelfSignedControllerCertificate()
		if err != nil {
			return controllerCert, err
		}
		if err = dataDir.SetControllerCertificate(controllerCert); err != nil {
			return controllerCert, err
		}
	}
	return
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
