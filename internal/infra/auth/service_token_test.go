package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	testServiceSecret   = "dGVzdC1zZXJ2aWNlLXNlY3JldC1rZXktMzItYnl0ZXMhIQ=="
	testServiceIssuer   = "gmhelper-api"
	testServiceAudience = "gmhelper-notify-api"
)

func TestNewServiceTokenProvider_Validation(t *testing.T) {
	// 1. Missing / invalid secret
	_, err := NewServiceTokenProvider(ServiceTokenProviderConfig{
		Secret:   "",
		Issuer:   testServiceIssuer,
		Audience: testServiceAudience,
	})
	if !errors.Is(err, ErrInvalidSecretKey) {
		t.Errorf("expected ErrInvalidSecretKey for empty secret, got: %v", err)
	}

	_, err = NewServiceTokenProvider(ServiceTokenProviderConfig{
		Secret:   "not-valid-base64!?",
		Issuer:   testServiceIssuer,
		Audience: testServiceAudience,
	})
	if !errors.Is(err, ErrInvalidSecretKey) {
		t.Errorf("expected ErrInvalidSecretKey for malformed base64, got: %v", err)
	}

	// 2. Missing issuer
	_, err = NewServiceTokenProvider(ServiceTokenProviderConfig{
		Secret:   testServiceSecret,
		Issuer:   "   ",
		Audience: testServiceAudience,
	})
	if !errors.Is(err, ErrMissingIssuer) {
		t.Errorf("expected ErrMissingIssuer for empty issuer, got: %v", err)
	}

	// 3. Missing audience
	_, err = NewServiceTokenProvider(ServiceTokenProviderConfig{
		Secret:   testServiceSecret,
		Issuer:   testServiceIssuer,
		Audience: "   ",
	})
	if !errors.Is(err, ErrMissingAudience) {
		t.Errorf("expected ErrMissingAudience for empty audience, got: %v", err)
	}

	// 4. Valid configuration with default TTL
	provider, err := NewServiceTokenProvider(ServiceTokenProviderConfig{
		Secret:   testServiceSecret,
		Issuer:   testServiceIssuer,
		Audience: testServiceAudience,
	})
	if err != nil {
		t.Fatalf("expected valid provider creation, got error: %v", err)
	}
	if provider.ttl != DefaultServiceTokenTTL {
		t.Errorf("expected default TTL %v, got %v", DefaultServiceTokenTTL, provider.ttl)
	}
}

func TestServiceTokenProvider_TokenGenerationAndClaims(t *testing.T) {
	beforeGen := time.Now().UTC().Add(-1 * time.Second)

	provider, err := NewServiceTokenProvider(ServiceTokenProviderConfig{
		Secret:   testServiceSecret,
		Issuer:   testServiceIssuer,
		Audience: testServiceAudience,
		TTL:      5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	tokenString, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("expected token generation success, got error: %v", err)
	}
	afterGen := time.Now().UTC().Add(1 * time.Second)

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}

	// 1. Verify Header (must be HS256)
	hdrBytes, err := decodeBase64URL(parts[0])
	if err != nil {
		t.Fatalf("failed to decode header: %v", err)
	}
	var hdr header
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		t.Fatalf("failed to parse header JSON: %v", err)
	}
	if hdr.Alg != "HS256" {
		t.Errorf("expected alg HS256, got: %s", hdr.Alg)
	}
	if hdr.Typ != "JWT" {
		t.Errorf("expected typ JWT, got: %s", hdr.Typ)
	}

	// 2. Verify Payload Claims
	payloadBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		t.Fatalf("failed to parse payload JSON: %v", err)
	}

	if claims.Sub != "gmhelper-api" {
		t.Errorf("expected sub 'gmhelper-api', got: %s", claims.Sub)
	}
	if claims.Name != "gmhelper-api" {
		t.Errorf("expected name 'gmhelper-api', got: %s", claims.Name)
	}
	if claims.SoapName != "gmhelper-api" {
		t.Errorf("expected SoapName 'gmhelper-api', got: %s", claims.SoapName)
	}
	if claims.GetUserID() != "gmhelper-api" {
		t.Errorf("expected GetUserID() 'gmhelper-api', got: %s", claims.GetUserID())
	}
	if claims.Role != "Service" || claims.SoapRole != "Service" {
		t.Errorf("expected role 'Service', got role=%s soapRole=%s", claims.Role, claims.SoapRole)
	}
	if claims.Iss != testServiceIssuer {
		t.Errorf("expected iss '%s', got: %s", testServiceIssuer, claims.Iss)
	}
	if !claims.MatchesAudience(testServiceAudience) {
		t.Errorf("expected aud '%s', got: %v", testServiceAudience, claims.Aud)
	}
	if claims.Iat < beforeGen.Unix() || claims.Iat > afterGen.Unix() {
		t.Errorf("expected iat close to now, got: %d", claims.Iat)
	}
	expectedExp := claims.Iat + int64((5 * time.Minute).Seconds())
	if claims.Exp != expectedExp {
		t.Errorf("expected exp %d, got: %d", expectedExp, claims.Exp)
	}
	if claims.Jti == "" {
		t.Error("expected non-empty unique jti")
	}

	// 3. Verify Token Cryptographically using JWTVerifier
	verifier := MustNewJWTVerifier(testServiceSecret, testServiceIssuer, testServiceAudience)
	principal, err := verifier.Verify(tokenString)
	if err != nil {
		t.Fatalf("expected JWTVerifier to verify service token, got error: %v", err)
	}
	if principal.UserID != "gmhelper-api" {
		t.Errorf("expected verified principal UserID 'gmhelper-api', got: %s", principal.UserID)
	}
	if principal.Role != "Service" {
		t.Errorf("expected verified principal Role 'Service', got: %s", principal.Role)
	}
}

func TestServiceTokenProvider_JtiUniqueness(t *testing.T) {
	provider, err := NewServiceTokenProvider(ServiceTokenProviderConfig{
		Secret:   testServiceSecret,
		Issuer:   testServiceIssuer,
		Audience: testServiceAudience,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	token1, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("failed to generate token1: %v", err)
	}
	token2, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("failed to generate token2: %v", err)
	}

	if token1 == token2 {
		t.Error("expected consecutively generated tokens to differ due to unique jti")
	}
}

func TestServiceTokenProvider_ContextCancelled(t *testing.T) {
	provider, _ := NewServiceTokenProvider(ServiceTokenProviderConfig{
		Secret:   testServiceSecret,
		Issuer:   testServiceIssuer,
		Audience: testServiceAudience,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.Token(ctx)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}
}

func TestServiceTokenProvider_GMHelperContractClaims(t *testing.T) {
	gmhelperIssuer := "GMHelperAPI"
	gmhelperAudience := "GMHelperClient"

	provider, err := NewServiceTokenProvider(ServiceTokenProviderConfig{
		Secret:   testServiceSecret,
		Issuer:   gmhelperIssuer,
		Audience: gmhelperAudience,
		TTL:      5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	tokenString, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(parts))
	}

	payloadBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		t.Fatalf("failed to parse JSON claims: %v", err)
	}

	if claims.Iss != "GMHelperAPI" {
		t.Errorf("expected iss 'GMHelperAPI', got '%s'", claims.Iss)
	}
	if !claims.MatchesAudience("GMHelperClient") {
		t.Errorf("expected aud 'GMHelperClient', got '%v'", claims.Aud)
	}
	if claims.Role != "Service" || claims.SoapRole != "Service" {
		t.Errorf("expected role 'Service', got role=%s soapRole=%s", claims.Role, claims.SoapRole)
	}
	if claims.Sub != "gmhelper-api" {
		t.Errorf("expected sub 'gmhelper-api', got '%s'", claims.Sub)
	}
	if claims.Name != "gmhelper-api" {
		t.Errorf("expected name 'gmhelper-api', got '%s'", claims.Name)
	}
}
