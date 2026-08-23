package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	testSecret   = "test-secret-key-32-bytes-long!!"
	testIssuer   = "gmhelper-api"
	testAudience = "gmhelper-notify-api"
)

func TestJWTVerifier_Verify_Success(t *testing.T) {
	verifier := NewJWTVerifier(testSecret, testIssuer, testAudience)

	token, err := GenerateToken(testSecret, testIssuer, testAudience, "user-123", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	principal, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("expected verification success, got error: %v", err)
	}

	if principal.UserID != "user-123" {
		t.Errorf("expected UserID user-123, got %s", principal.UserID)
	}
	if principal.Role != "admin" {
		t.Errorf("expected Role admin, got %s", principal.Role)
	}
}

func TestJWTVerifier_Verify_ExpiredToken(t *testing.T) {
	verifier := NewJWTVerifier(testSecret, testIssuer, testAudience)

	// Token expired 10 minutes ago
	token, err := GenerateToken(testSecret, testIssuer, testAudience, "user-123", "user", -10*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = verifier.Verify(token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestJWTVerifier_Verify_InvalidSignature(t *testing.T) {
	verifier := NewJWTVerifier(testSecret, testIssuer, testAudience)

	// Signed with wrong secret
	token, err := GenerateToken("wrong-secret-key", testIssuer, testAudience, "user-123", "user", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = verifier.Verify(token)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestJWTVerifier_Verify_TamperedPayload(t *testing.T) {
	verifier := NewJWTVerifier(testSecret, testIssuer, testAudience)

	token, _ := GenerateToken(testSecret, testIssuer, testAudience, "user-123", "user", 15*time.Minute)
	parts := strings.Split(token, ".")

	// Tamper payload to elevate role to admin without re-signing
	tamperedClaims := Claims{
		Sub:  "user-123",
		Role: "superadmin",
		Iss:  testIssuer,
		Aud:  testAudience,
		Exp:  time.Now().Add(15 * time.Minute).Unix(),
	}
	b, _ := json.Marshal(tamperedClaims)
	tamperedPayload := base64.RawURLEncoding.EncodeToString(b)

	tamperedToken := parts[0] + "." + tamperedPayload + "." + parts[2]

	_, err := verifier.Verify(tamperedToken)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature on tampered token, got %v", err)
	}
}

func TestJWTVerifier_Verify_WrongIssuer(t *testing.T) {
	verifier := NewJWTVerifier(testSecret, testIssuer, testAudience)

	token, _ := GenerateToken(testSecret, "evil-issuer", testAudience, "user-123", "user", 15*time.Minute)

	_, err := verifier.Verify(token)
	if !errors.Is(err, ErrInvalidIssuer) {
		t.Fatalf("expected ErrInvalidIssuer, got %v", err)
	}
}

func TestJWTVerifier_Verify_WrongAudience(t *testing.T) {
	verifier := NewJWTVerifier(testSecret, testIssuer, testAudience)

	token, _ := GenerateToken(testSecret, testIssuer, "other-service", "user-123", "user", 15*time.Minute)

	_, err := verifier.Verify(token)
	if !errors.Is(err, ErrInvalidAudience) {
		t.Fatalf("expected ErrInvalidAudience, got %v", err)
	}
}

func TestJWTVerifier_Verify_AlgorithmConfusionProtection(t *testing.T) {
	verifier := NewJWTVerifier(testSecret, testIssuer, testAudience)

	// Create header with "none" alg
	hdr := header{Alg: "none", Typ: "JWT"}
	hdrBytes, _ := json.Marshal(hdr)
	claims := Claims{Sub: "user-1", Iss: testIssuer, Aud: testAudience}
	claimsBytes, _ := json.Marshal(claims)

	noneToken := encodeBase64URL(hdrBytes) + "." + encodeBase64URL(claimsBytes) + "."

	_, err := verifier.Verify(noneToken)
	if !errors.Is(err, ErrUnsupportedAlg) {
		t.Fatalf("expected ErrUnsupportedAlg for 'none' algorithm, got %v", err)
	}
}

func TestJWTVerifier_Verify_MissingSubject(t *testing.T) {
	verifier := NewJWTVerifier(testSecret, testIssuer, testAudience)

	// Token with empty sub and user_id
	token, _ := GenerateToken(testSecret, testIssuer, testAudience, "", "user", 15*time.Minute)

	_, err := verifier.Verify(token)
	if !errors.Is(err, ErrMissingSubject) {
		t.Fatalf("expected ErrMissingSubject, got %v", err)
	}
}

func TestJWTVerifier_Verify_MissingExpiration(t *testing.T) {
	verifier := NewJWTVerifier(testSecret, testIssuer, testAudience)

	// Token with Exp = 0 (no exp claim)
	hdr := header{Alg: "HS256", Typ: "JWT"}
	hdrBytes, _ := json.Marshal(hdr)
	claims := Claims{Sub: "user-1", Iss: testIssuer, Aud: testAudience, Exp: 0}
	claimsBytes, _ := json.Marshal(claims)

	signingInput := encodeBase64URL(hdrBytes) + "." + encodeBase64URL(claimsBytes)
	mac := computeHMACSHA256([]byte(signingInput), []byte(testSecret))
	tokenWithoutExp := signingInput + "." + encodeBase64URL(mac)

	_, err := verifier.Verify(tokenWithoutExp)
	if !errors.Is(err, ErrMissingExpiration) {
		t.Fatalf("expected ErrMissingExpiration for token without exp claim, got %v", err)
	}
}
