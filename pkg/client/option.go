package client

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/cirruslabs/orchard/internal/netconstants"
)

type Option func(*Client)

func WithAddress(address string) Option {
	return func(client *Client) {
		client.address = address
	}
}

func WithTrustedCertificate(cert *x509.Certificate) Option {
	return func(client *Client) {
		// Check that the API is accessible
		privatePool := x509.NewCertPool()
		privatePool.AddCert(cert)

		client.tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS13,
			RootCAs:    privatePool,
			ServerName: netconstants.DefaultControllerServerName,
		}
	}
}

func WithCredentials(serviceAccountName string, serviceAccountToken string) Option {
	return func(client *Client) {
		client.serviceAccountName = serviceAccountName
		client.serviceAccountToken = serviceAccountToken
	}
}
