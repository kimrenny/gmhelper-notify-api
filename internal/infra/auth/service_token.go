package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultServiceTokenTTL = 5 * time.Minute
	DefaultServiceSubject  = "gmhelper-notify-api"
	DefaultServiceRole     = "Service"
)

var (
	ErrMissingIssuer   = errors.New("service token issuer cannot be empty")
	ErrMissingAudience = errors.New("service token audience cannot be empty")
)

// ServiceTokenProvider defines the interface for generating machine-to-machine service tokens.
type ServiceTokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// ServiceTokenProviderConfig contains the configuration parameters for JWTServiceTokenProvider.
type ServiceTokenProviderConfig struct {
	Secret   string
	Issuer   string
	Audience string
	TTL      time.Duration
}

// JWTServiceTokenProvider implements ServiceTokenProvider using HMAC-SHA256 (HS256) JWTs.
type JWTServiceTokenProvider struct {
	secret   []byte
	issuer   string
	audience string
	ttl      time.Duration
	nowFunc  func() time.Time
}

// NewServiceTokenProvider constructs and validates a JWTServiceTokenProvider.
func NewServiceTokenProvider(cfg ServiceTokenProviderConfig) (*JWTServiceTokenProvider, error) {
	key, err := DecodeSecretKey(cfg.Secret)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSecretKey, err)
	}

	issuer := strings.TrimSpace(cfg.Issuer)
	if issuer == "" {
		return nil, ErrMissingIssuer
	}

	audience := strings.TrimSpace(cfg.Audience)
	if audience == "" {
		return nil, ErrMissingAudience
	}

	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = DefaultServiceTokenTTL
	}

	return &JWTServiceTokenProvider{
		secret:   key,
		issuer:   issuer,
		audience: audience,
		ttl:      ttl,
		nowFunc:  time.Now,
	}, nil
}

// Token generates a fresh signed HS256 JWT representing the machine service identity.
func (p *JWTServiceTokenProvider) Token(ctx context.Context) (string, error) {
	if ctx != nil && ctx.Err() != nil {
		return "", ctx.Err()
	}

	now := p.nowFunc().UTC()
	claims := Claims{
		Sub:      DefaultServiceSubject,
		UserID:   DefaultServiceSubject,
		SoapName: DefaultServiceSubject,
		Role:     DefaultServiceRole,
		SoapRole: DefaultServiceRole,
		Iss:      p.issuer,
		Aud:      p.audience,
		Exp:      now.Add(p.ttl).Unix(),
		Iat:      now.Unix(),
		Nbf:      now.Unix(),
		Jti:      uuid.NewString(),
	}

	return GenerateTokenWithClaims(p.secret, claims)
}

// SetNowFunc sets a custom time provider function (useful for deterministic testing).
func (p *JWTServiceTokenProvider) SetNowFunc(nowFunc func() time.Time) {
	if nowFunc != nil {
		p.nowFunc = nowFunc
	}
}
