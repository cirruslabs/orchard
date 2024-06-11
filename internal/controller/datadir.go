package controller

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
)

type DataDir struct {
	path string
}

func NewDataDir(path string) (*DataDir, error) {
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory at path %s: %w",
			path, err)
	}

	return &DataDir{
		path: path,
	}, nil
}

func (dataDir *DataDir) ControllerCertificate() (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(dataDir.ControllerCertificatePath(), dataDir.ControllerKeyPath())
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load controller's certificate and key: %w", err)
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
		return fmt.Errorf("failed to set controller's certificate: PKCS #8 marshalling failed: %w",
			err)
	}

	privateKeyPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	err = os.WriteFile(dataDir.ControllerCertificatePath(), certPEMBytes, 0600)
	if err != nil {
		return fmt.Errorf("failed to write controller's certificate: %w", err)
	}
	err = os.WriteFile(dataDir.ControllerKeyPath(), privateKeyPEMBytes, 0600)
	if err != nil {
		return fmt.Errorf("failed to write controller's key: %w", err)
	}

	return nil
}

func (dataDir *DataDir) SSHHostKey() (ssh.Signer, error) {
	hostKeyBytes, err := os.ReadFile(dataDir.SSHHostKeyPath())
	if err != nil {
		return nil, err
	}

	return ssh.ParsePrivateKey(hostKeyBytes)
}

func (dataDir *DataDir) SetSSHHostKey(privateKey crypto.PrivateKey) error {
	pemBlock, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		return err
	}

	return os.WriteFile(dataDir.SSHHostKeyPath(), pem.EncodeToMemory(pemBlock), 0600)
}

func (dataDir *DataDir) DBPath() string {
	return filepath.Join(dataDir.path, "db")
}

func (dataDir *DataDir) ControllerCertificatePath() string {
	return filepath.Join(dataDir.path, "controller.crt")
}

func (dataDir *DataDir) ControllerKeyPath() string {
	return filepath.Join(dataDir.path, "controller.key")
}

func (dataDir *DataDir) SSHHostKeyPath() string {
	return filepath.Join(dataDir.path, "ssh_host_ed25519_key")
}

func (dataDir *DataDir) Initialized() (bool, error) {
	dataDirEntries, err := os.ReadDir(dataDir.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("failed to read data directory contents at path %s: %w",
			dataDir.path, err)
	}

	return len(dataDirEntries) != 0, nil
}
