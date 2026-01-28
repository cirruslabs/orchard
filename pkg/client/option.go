package client

import (
	"crypto/x509"

	"github.com/cirruslabs/orchard/internal/dialer"
)

type Option func(*Client)

func WithAddress(address string) Option {
	return func(client *Client) {
		client.address = address
	}
}

func WithTrustedCertificate(trustedCertificate *x509.Certificate) Option {
	return func(client *Client) {
		client.trustedCertificate = trustedCertificate
	}
}

func WithCredentials(serviceAccountName string, serviceAccountToken string) Option {
	return func(client *Client) {
		client.serviceAccountName = serviceAccountName
		client.serviceAccountToken = serviceAccountToken
	}
}

func WithDialer(dialer dialer.Dialer) Option {
	return func(client *Client) {
		client.dialer = dialer
	}
}
