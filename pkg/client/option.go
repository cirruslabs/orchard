package client

import (
	"crypto/x509"
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
