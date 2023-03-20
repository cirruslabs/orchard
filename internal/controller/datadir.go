package controller

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrDataDirError = errors.New("controller's data directory operation error")

type DataDir struct {
	path string
}

func NewDataDir(path string) (*DataDir, error) {
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, fmt.Errorf("%w: failed to create data directory at path %s: %v",
			ErrDataDirError, path, err)
	}

	return &DataDir{
		path: path,
	}, nil
}

func (dataDir *DataDir) ControllerCertificate() (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(dataDir.ControllerCertificatePath(), dataDir.ControllerKeyPath())
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: failed to load controller's certificate and key: %v",
			ErrDataDirError, err)
	}

	return cert, nil
}

func (dataDir *DataDir) SetControllerCertificate(certificate tls.Certificate) error {
	certPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificate.Certificate[0],
	})

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		return fmt.Errorf("%w: failed to set controller's certificate: PKCS #8 marshalling failed: %v",
			ErrDataDirError, err)
	}

	privateKeyPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	err = os.WriteFile(dataDir.ControllerCertificatePath(), certPEMBytes, 0600)
	if err != nil {
		return fmt.Errorf("%w: failed to write controller's certificate: %v", ErrDataDirError, err)
	}
	err = os.WriteFile(dataDir.ControllerKeyPath(), privateKeyPEMBytes, 0600)
	if err != nil {
		return fmt.Errorf("%w: failed to write controller's key: %v", ErrDataDirError, err)
	}

	return nil
}

func (dataDir *DataDir) DBPath() string {
	return filepath.Join(dataDir.path, "db")
}

func (dataDir *DataDir) ControllerCertificateExists() bool {
	return fileExist(dataDir.ControllerCertificatePath()) && fileExist(dataDir.ControllerKeyPath())
}

func fileExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (dataDir *DataDir) ControllerCertificatePath() string {
	return filepath.Join(dataDir.path, "controller.crt")
}

func (dataDir *DataDir) ControllerKeyPath() string {
	return filepath.Join(dataDir.path, "controller.key")
}

func (dataDir *DataDir) Initialized() (bool, error) {
	dataDirEntries, err := os.ReadDir(dataDir.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("%w: failed to read data directory contents at path %s: %v",
			ErrDataDirError, dataDir.path, err)
	}

	return len(dataDirEntries) != 0, nil
}
