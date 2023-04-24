package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/config"
	"github.com/cirruslabs/orchard/rpc"
	"golang.org/x/net/websocket"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"io"
	"net/http"
	"net/url"
)

var (
	ErrFailed       = errors.New("API client failed")
	ErrInvalidState = errors.New("invalid state")
)

type Client struct {
	address   string
	insecure  bool
	tlsConfig *tls.Config

	httpClient *http.Client
	baseURL    *url.URL

	serviceAccountName  string
	serviceAccountToken string
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
		configHandle, err := config.NewHandle()
		if err != nil {
			return nil, err
		}

		defaultContext, err := configHandle.DefaultContext()
		if err != nil {
			return nil, err
		}

		client.address = defaultContext.URL
		client.serviceAccountName = defaultContext.ServiceAccountName
		client.serviceAccountToken = defaultContext.ServiceAccountToken

		tlsConfig, err := defaultContext.TLSConfig()
		if err != nil {
			return nil, err
		}
		client.tlsConfig = tlsConfig
	}

	// Instantiate client
	client.httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: client.tlsConfig,
		},
	}

	url, err := url.Parse(client.address)
	if err != nil {
		return nil, err
	}
	client.baseURL = url

	// Figure out if HTTP (insecure) or HTTPS (secure) was requested,
	// so we can further adapt for gRPC and WebSocket usage patterns
	switch client.baseURL.Scheme {
	case "http":
		client.insecure = true
	case "https":
		// do nothing, we're secure by default
	default:
		return nil, fmt.Errorf("%w: only http https schemes are supported, got %s",
			ErrFailed, client.baseURL.Scheme)
	}

	return client, nil
}

func (client *Client) GRPCTarget() string {
	return client.baseURL.Host
}

func (client *Client) GRPCTransportCredentials() credentials.TransportCredentials {
	if client.insecure {
		return insecure.NewCredentials()
	}

	return credentials.NewTLS(client.tlsConfig)
}

func (client *Client) GPRCMetadata() metadata.MD {
	result := map[string]string{}

	if client.serviceAccountName != "" && client.serviceAccountToken != "" {
		result = map[string]string{
			rpc.MetadataServiceAccountNameKey:  client.serviceAccountName,
			rpc.MetadataServiceAccountTokenKey: client.serviceAccountToken,
		}
	}

	return metadata.New(result)
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

	endpointURL, err := client.parsePath(path)
	if err != nil {
		return err
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

	if client.serviceAccountName != "" && client.serviceAccountToken != "" {
		request.SetBasicAuth(client.serviceAccountName, client.serviceAccountToken)
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w to make a request: %v", ErrFailed, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w to make a request: %d %s%s",
			ErrFailed, response.StatusCode, http.StatusText(response.StatusCode),
			detailsFromErrorResponseBody(response.Body))
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

func detailsFromErrorResponseBody(body io.Reader) string {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return ""
	}

	var errorResponse struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(bodyBytes, &errorResponse); err != nil {
		return ""
	}

	if errorResponse.Message != "" {
		return fmt.Sprintf(" (%s)", errorResponse.Message)
	}

	return ""
}

func (client *Client) wsRequest(
	_ context.Context,
	path string,
	params map[string]string,
) (*websocket.Conn, error) {
	endpointURL, err := client.parsePath(path)
	if err != nil {
		return nil, err
	}

	// Adapt HTTP scheme to WebSocket scheme
	if client.insecure {
		endpointURL.Scheme = "ws"
	} else {
		endpointURL.Scheme = "wss"
	}

	values := endpointURL.Query()
	for key, value := range params {
		values.Set(key, value)
	}
	endpointURL.RawQuery = values.Encode()

	config, err := websocket.NewConfig(endpointURL.String(), "http://127.0.0.1/")
	if err != nil {
		return nil, fmt.Errorf("%w to create WebSocket configuration: %v", ErrFailed, err)
	}

	if client.serviceAccountName != "" && client.serviceAccountToken != "" {
		authPlain := fmt.Sprintf("%s:%s", client.serviceAccountName, client.serviceAccountToken)
		authEncoded := base64.StdEncoding.EncodeToString([]byte(authPlain))
		config.Header.Add("Authorization", fmt.Sprintf("Basic %s", authEncoded))
	}

	config.TlsConfig = client.tlsConfig

	return websocket.DialConfig(config)
}

func (client *Client) parsePath(path string) (*url.URL, error) {
	endpointURL, err := url.Parse("v1/" + path)
	if err != nil {
		return nil, fmt.Errorf("%w to parse API endpoint path: %v", ErrFailed, err)
	}

	return &url.URL{
		Scheme:  client.baseURL.Scheme,
		User:    client.baseURL.User,
		Host:    client.baseURL.Host,
		Path:    endpointURL.Path,
		RawPath: endpointURL.RawPath,
	}, nil
}

func (client *Client) Check(ctx context.Context) error {
	return client.request(ctx, http.MethodGet, "/", nil, nil, nil)
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

func (client *Client) ServiceAccounts() *ServiceAccountsService {
	return &ServiceAccountsService{
		client: client,
	}
}

func (client *Client) Controller() *ControllerService {
	return &ControllerService{
		client: client,
	}
}
