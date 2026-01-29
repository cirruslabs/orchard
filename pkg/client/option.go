package client

import (
	"crypto/x509"
	"fmt"
	"strings"

	"github.com/cirruslabs/orchard/internal/dialer"
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
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

type ListInput struct {
	Filters []v1.Filter
}

type ListOption func(params map[string]string)

func WithListFilters(filters ...v1.Filter) ListOption {
	return func(params map[string]string) {
		if len(filters) == 0 {
			return
		}

		var pairs []string

		for _, filter := range filters {
			pairs = append(pairs, fmt.Sprintf("%s=%s", filter.Path, filter.Value))
		}

		params["filter"] = strings.Join(pairs, ",")
	}
}
