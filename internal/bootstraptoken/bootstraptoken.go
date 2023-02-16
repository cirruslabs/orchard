package bootstraptoken

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrFailedToCreateBootstrapToken = errors.New("failed to create bootstrap token")
	ErrInvalidBootstrapTokenFormat  = errors.New("invalid bootstrap token format")

	encoding = base64.RawURLEncoding
)

const (
	versionPrefix = "orchard-bootstrap-token-v"
	version       = 0
)

type BootstrapToken struct {
	version             int
	certificate         *x509.Certificate
	rawCertificate      []byte
	serviceAccountName  string
	serviceAccountToken string
}

func New(rawCertificate []byte, serviceAccountName string, serviceAccountToken string) (*BootstrapToken, error) {
	if len(rawCertificate) == 0 {
		return nil, fmt.Errorf("%w: empty certificate", ErrFailedToCreateBootstrapToken)
	}

	if serviceAccountName == "" {
		return nil, fmt.Errorf("%w: empty service account name", ErrFailedToCreateBootstrapToken)
	}

	if serviceAccountToken == "" {
		return nil, fmt.Errorf("%w: empty service account token", ErrFailedToCreateBootstrapToken)
	}

	// Parse certificate
	block, _ := pem.Decode(rawCertificate)
	if block == nil {
		return nil, fmt.Errorf("%w: failed to parse certificate: expected a PEM format",
			ErrFailedToCreateBootstrapToken)
	}

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse certificate: %v",
			ErrFailedToCreateBootstrapToken, err)
	}

	return &BootstrapToken{
		version:             version,
		certificate:         certificate,
		serviceAccountName:  serviceAccountName,
		serviceAccountToken: serviceAccountToken,
		rawCertificate:      rawCertificate,
	}, nil
}

func NewFromString(rawBootstrapToken string) (*BootstrapToken, error) {
	splits := strings.Split(rawBootstrapToken, ".")

	currentVersionString := fmt.Sprintf("%s%d", versionPrefix, version)

	if splits[0] != currentVersionString {
		return nil, fmt.Errorf("%w: invalid version string or unsupported version",
			ErrInvalidBootstrapTokenFormat)
	}

	if len(splits) < 3 {
		return nil, fmt.Errorf("%w: missing service account credentials", ErrInvalidBootstrapTokenFormat)
	}

	if len(splits) < 4 {
		return nil, fmt.Errorf("%w: missing certificate", ErrInvalidBootstrapTokenFormat)
	}

	if len(splits) > 4 {
		return nil, fmt.Errorf("%w: extraneous data", ErrInvalidBootstrapTokenFormat)
	}

	serviceAccountName, err := encoding.DecodeString(splits[1])
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode service account name: %v",
			ErrInvalidBootstrapTokenFormat, err)
	}
	serviceAccountToken, err := encoding.DecodeString(splits[2])
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode service account token: %v",
			ErrInvalidBootstrapTokenFormat, err)
	}
	rawCertificate, err := encoding.DecodeString(splits[3])
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode certificate: %v",
			ErrInvalidBootstrapTokenFormat, err)
	}

	// Parse certificate
	block, _ := pem.Decode(rawCertificate)

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse certificate: %v",
			ErrFailedToCreateBootstrapToken, err)
	}

	return &BootstrapToken{
		version:             version,
		certificate:         certificate,
		rawCertificate:      rawCertificate,
		serviceAccountName:  string(serviceAccountName),
		serviceAccountToken: string(serviceAccountToken),
	}, nil
}

func (bt *BootstrapToken) String() string {
	return fmt.Sprintf("%s%d.%s.%s.%s",
		versionPrefix,
		version,
		encoding.EncodeToString([]byte(bt.serviceAccountName)),
		encoding.EncodeToString([]byte(bt.serviceAccountToken)),
		encoding.EncodeToString(bt.rawCertificate),
	)
}

func (bt *BootstrapToken) ServiceAccountName() string {
	return bt.serviceAccountName
}

func (bt *BootstrapToken) ServiceAccountToken() string {
	return bt.serviceAccountToken
}

func (bt *BootstrapToken) Certificate() *x509.Certificate {
	return bt.certificate
}
