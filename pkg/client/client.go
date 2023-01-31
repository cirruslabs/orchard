package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

var (
	ErrFailed = errors.New("API client failed")
)

type Client struct {
	address   string
	tlsConfig *tls.Config

	httpClient *http.Client
	baseURL    *url.URL
}

type Config struct {
	Address   string
	TLSConfig *tls.Config
}

func New(opts ...Option) (*Client, error) {
	client := &Client{}

	// Apply options
	for _, opt := range opts {
		opt(client)
	}

	// Apply defaults
	if client.address == "" {
		client.address = "http://127.0.0.1:6120"
	}

	// Instantiate client
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: client.tlsConfig,
		},
	}

	url, err := url.Parse(client.address)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpClient: httpClient,
		baseURL:    url,
	}, nil
}

func (client *Client) request(
	ctx context.Context,
	method string,
	path string,
	in interface{},
	out interface{},
	params map[string]string,
) error {
	var body io.Reader

	if in != nil {
		jsonBytes, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("%w to marshal request body: %v", ErrFailed, err)
		}

		body = bytes.NewBuffer(jsonBytes)
	}

	endpointURL, err := url.Parse("v1/" + path)
	if err != nil {
		return fmt.Errorf("%w to parse API endpoint path: %v", ErrFailed, err)
	}

	endpointURL = &url.URL{
		Scheme:  client.baseURL.Scheme,
		User:    client.baseURL.User,
		Host:    client.baseURL.Host,
		Path:    endpointURL.Path,
		RawPath: endpointURL.RawPath,
	}

	values := endpointURL.Query()
	for key, value := range params {
		values.Set(key, value)
	}
	endpointURL.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, method, endpointURL.String(), body)
	if err != nil {
		return fmt.Errorf("%w instantiate a request: %v", ErrFailed, err)
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w to make a request: %v", ErrFailed, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w to make a request: %d %s",
			ErrFailed, response.StatusCode, http.StatusText(response.StatusCode))
	}

	if out != nil {
		bodyBytes, err := io.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("%w to read response body: %v", ErrFailed, err)
		}

		if err := json.Unmarshal(bodyBytes, out); err != nil {
			return fmt.Errorf("%w to unmarshal response body: %v", ErrFailed, err)
		}
	}

	return nil
}

func (client *Client) Workers() *WorkersService {
	return &WorkersService{
		client: client,
	}
}

func (client *Client) VMs() *VMsService {
	return &VMsService{
		client: client,
	}
}
