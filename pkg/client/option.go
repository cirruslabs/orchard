package client

import (
	"context"
	"crypto/x509"
	"net"
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

func WithDialContext(dialContext func(ctx context.Context, network, addr string) (net.Conn, error)) Option {
	return func(client *Client) {
		client.dialContext = dialContext
	}
}
