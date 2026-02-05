package vmtempauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	SigningKeySizeBytes = 32

	JWTHeaderAlgHS256 = "HS256"
	JWTHeaderTypJWT   = "JWT"

	AccessTokenAudience = "orchard-vm-access"
	AccessTokenIssuer   = "orchard-controller"

	ScopeVMPortForward = "vm:port-forward"
	ScopeVMIP          = "vm:ip"
	ScopeVMSSHJumpbox  = "vm:ssh-jumpbox"

	PortsAny = "*"

	DefaultTTL = 24 * time.Hour
	MaxTTL     = 30 * 24 * time.Hour

	SSHUsername = "token"
)

var (
	ErrInvalidSigningKey  = errors.New("invalid signing key")
	ErrMalformedToken     = errors.New("malformed token")
	ErrInvalidTokenHeader = errors.New("invalid token header")
	ErrInvalidTokenClaims = errors.New("invalid token claims")
	ErrSignatureMismatch  = errors.New("token signature mismatch")
	ErrTokenExpired       = errors.New("token expired")
	ErrTokenNotYetValid   = errors.New("token not yet valid")
	ErrInvalidTTL         = errors.New("invalid access token TTL")
	encoding              = base64.RawURLEncoding
	requiredScopes        = []string{ScopeVMPortForward, ScopeVMIP, ScopeVMSSHJumpbox}
)

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type Claims struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Audience  []string `json:"aud"`
	IssuedAt  int64    `json:"iat"`
	NotBefore int64    `json:"nbf"`
	ExpiresAt int64    `json:"exp"`
	JTI       string   `json:"jti"`

	VMUID  string   `json:"vm_uid"`
	VMName string   `json:"vm_name,omitempty"`
	Scopes []string `json:"scopes"`
	Ports  string   `json:"ports,omitempty"`
}

func (claims Claims) HasScope(scope string) bool {
	return slices.Contains(claims.Scopes, scope)
}

func (claims Claims) CanAccessVM(vmUID string) bool {
	return claims.VMUID != "" && vmUID != "" && claims.VMUID == vmUID
}

type IssueInput struct {
	Issuer  string
	Subject string
	VMUID   string
	VMName  string
	TTL     time.Duration
	Now     time.Time
}

type IssueOutput struct {
	Token     string
	Claims    Claims
	ExpiresAt time.Time
}

func NewSigningKey() ([]byte, error) {
	key := make([]byte, SigningKeySizeBytes)

	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	return key, nil
}

func NormalizeTTL(ttlSeconds *uint64) (time.Duration, error) {
	if ttlSeconds == nil {
		return DefaultTTL, nil
	}

	if *ttlSeconds == 0 {
		return 0, fmt.Errorf("%w: TTL cannot be zero", ErrInvalidTTL)
	}

	ttl := time.Duration(*ttlSeconds) * time.Second

	if ttl > MaxTTL {
		return 0, fmt.Errorf("%w: maximum allowed TTL is %s", ErrInvalidTTL, MaxTTL)
	}

	return ttl, nil
}

func Issue(signingKey []byte, input IssueInput) (*IssueOutput, error) {
	if len(signingKey) != SigningKeySizeBytes {
		return nil, ErrInvalidSigningKey
	}

	if input.Subject == "" {
		return nil, fmt.Errorf("%w: missing subject", ErrInvalidTokenClaims)
	}
	if input.VMUID == "" {
		return nil, fmt.Errorf("%w: missing vm_uid", ErrInvalidTokenClaims)
	}

	ttl := input.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if ttl < 0 || ttl > MaxTTL {
		return nil, fmt.Errorf("%w: unsupported TTL %s", ErrInvalidTTL, ttl)
	}

	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	issuer := input.Issuer
	if issuer == "" {
		issuer = AccessTokenIssuer
	}

	claims := Claims{
		Issuer:    issuer,
		Subject:   input.Subject,
		Audience:  []string{AccessTokenAudience},
		IssuedAt:  now.Unix(),
		NotBefore: now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		JTI:       uuid.NewString(),
		VMUID:     input.VMUID,
		VMName:    input.VMName,
		Scopes:    append([]string{}, requiredScopes...),
		Ports:     PortsAny,
	}

	token, err := encodeAndSign(signingKey, claims)
	if err != nil {
		return nil, err
	}

	return &IssueOutput{
		Token:     token,
		Claims:    claims,
		ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
	}, nil
}

func Verify(signingKey []byte, token string, now time.Time) (*Claims, error) {
	if len(signingKey) != SigningKeySizeBytes {
		return nil, ErrInvalidSigningKey
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrMalformedToken
	}

	splits := strings.Split(token, ".")
	if len(splits) != 3 {
		return nil, ErrMalformedToken
	}

	headerRaw, err := encoding.DecodeString(splits[0])
	if err != nil {
		return nil, ErrMalformedToken
	}

	var tokenHeader header

	if err := json.Unmarshal(headerRaw, &tokenHeader); err != nil {
		return nil, ErrMalformedToken
	}

	if tokenHeader.Alg != JWTHeaderAlgHS256 || tokenHeader.Typ != JWTHeaderTypJWT {
		return nil, ErrInvalidTokenHeader
	}

	signedPart := fmt.Sprintf("%s.%s", splits[0], splits[1])
	expectedSignature := sign(signingKey, signedPart)

	presentedSignature, err := encoding.DecodeString(splits[2])
	if err != nil {
		return nil, ErrMalformedToken
	}

	if subtle.ConstantTimeCompare(expectedSignature, presentedSignature) != 1 {
		return nil, ErrSignatureMismatch
	}

	claimsRaw, err := encoding.DecodeString(splits[1])
	if err != nil {
		return nil, ErrMalformedToken
	}

	var claims Claims

	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		return nil, ErrMalformedToken
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}

	if err := validateClaims(claims, now); err != nil {
		return nil, err
	}

	return &claims, nil
}

func encodeAndSign(signingKey []byte, claims Claims) (string, error) {
	headerJSON, err := json.Marshal(header{
		Alg: JWTHeaderAlgHS256,
		Typ: JWTHeaderTypJWT,
	})
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signedPart := fmt.Sprintf("%s.%s",
		encoding.EncodeToString(headerJSON),
		encoding.EncodeToString(claimsJSON),
	)

	signature := sign(signingKey, signedPart)

	return fmt.Sprintf("%s.%s", signedPart, encoding.EncodeToString(signature)), nil
}

func sign(signingKey []byte, signedPart string) []byte {
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(signedPart))

	return mac.Sum(nil)
}

func validateClaims(claims Claims, now time.Time) error {
	if claims.Issuer == "" || claims.Subject == "" || claims.JTI == "" || claims.VMUID == "" {
		return ErrInvalidTokenClaims
	}

	if claims.IssuedAt == 0 || claims.NotBefore == 0 || claims.ExpiresAt == 0 {
		return ErrInvalidTokenClaims
	}

	if !slices.Contains(claims.Audience, AccessTokenAudience) {
		return ErrInvalidTokenClaims
	}

	if claims.NotBefore > now.Unix() {
		return ErrTokenNotYetValid
	}
	if claims.ExpiresAt <= now.Unix() {
		return ErrTokenExpired
	}

	for _, requiredScope := range requiredScopes {
		if !claims.HasScope(requiredScope) {
			return ErrInvalidTokenClaims
		}
	}

	if claims.Ports != "" && claims.Ports != PortsAny {
		return ErrInvalidTokenClaims
	}

	return nil
}
