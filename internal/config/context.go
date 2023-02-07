package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/cirruslabs/orchard/internal/controller"
)

type Context struct {
	URL         string `yaml:"url,omitempty"`
	Certificate Base64 `yaml:"certificate,omitempty"`
}

func (context *Context) TLSConfig() (*tls.Config, error) {
	if len(context.Certificate) == 0 {
		return nil, nil
	}

	privatePool := x509.NewCertPool()

	if ok := privatePool.AppendCertsFromPEM(context.Certificate); !ok {
		return nil, fmt.Errorf("%w: failed to load context's certificate", ErrConfigReadFailed)
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		ServerName: controller.DefaultServerName,
		RootCAs:    privatePool,
	}, nil
}
