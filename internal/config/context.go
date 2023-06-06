package config

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

type Context struct {
	URL                 string `yaml:"url,omitempty"`
	Certificate         Base64 `yaml:"certificate,omitempty"`
	ServiceAccountName  string `yaml:"serviceAccountName,omitempty"`
	ServiceAccountToken string `yaml:"serviceAccountToken,omitempty"`
}

func (context *Context) TrustedCertificate() (*x509.Certificate, error) {
	if len(context.Certificate) == 0 {
		return nil, nil
	}

	block, _ := pem.Decode(context.Certificate)
	if block == nil {
		return nil, fmt.Errorf("%w: failed to load context's certificate: no PEM data found",
			ErrConfigReadFailed)
	}

	trustedCertificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load context's certificate: %v",
			ErrConfigReadFailed, err)
	}

	return trustedCertificate, nil
}
