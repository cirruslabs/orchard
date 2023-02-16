package client

import "crypto/tls"

type Option func(*Client)

func WithAddress(address string) Option {
	return func(client *Client) {
		client.address = address
	}
}

func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(client *Client) {
		client.tlsConfig = tlsConfig
	}
}

func WithCredentials(serviceAccountName string, serviceAccountToken string) Option {
	return func(client *Client) {
		client.serviceAccountName = serviceAccountName
		client.serviceAccountToken = serviceAccountToken
	}
}
