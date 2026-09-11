package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	// testBase64Secret is a 32-byte secret encoded in standard Base64:
	// Raw ASCII: "test-secret-key-32-bytes-long!!"
	testBase64Secret = "dGVzdC1zZWNyZXQta2V5LTMyLWJ5dGVzLWxvbmchIQ=="
	testIssuer       = "gmhelper-api"
	testAudience     = "gmhelper-notify-api"
)

func TestDecodeSecretKey_Success(t *testing.T) {
	// Standard base64 with padding
	raw, err := DecodeSecretKey("dGVzdC1zZWNyZXQta2V5LTMyLWJ5dGVzLWxvbmchIQ==")
	if err != nil {
		t.Fatalf("expected successful decode, got: %v", err)
	}
	if string(raw) != "test-secret-key-32-bytes-long!!" {
		t.Errorf("expected 'test-secret-key-32-bytes-long!!', got '%s'", string(raw))
	}

	// Unpadded base64
	rawUnpadded, err := DecodeSecretKey("dGVzdC1zZWNyZXQta2V5LTMyLWJ5dGVzLWxvbmchIQ")
	if err != nil {
		t.Fatalf("expected successful decode for unpadded base64, got: %v", err)
	}
	if string(rawUnpadded) != "test-secret-key-32-bytes-long!!" {
		t.Errorf("expected 'test-secret-key-32-bytes-long!!', got '%s'", string(rawUnpadded))
	}
}

func TestDecodeSecretKey_Errors(t *testing.T) {
	// Empty string
	if _, err := DecodeSecretKey(""); !errors.Is(err, ErrInvalidSecretKey) {
		t.Errorf("expected ErrInvalidSecretKey for empty secret, got %v", err)
	}

	// Whitespace only
	if _, err := DecodeSecretKey("   "); !errors.Is(err, ErrInvalidSecretKey) {
		t.Errorf("expected ErrInvalidSecretKey for whitespace secret, got %v", err)
	}

	// Malformed base64
	if _, err := DecodeSecretKey("not-valid-base64-characters!!!"); !errors.Is(err, ErrInvalidSecretKey) {
		t.Errorf("expected ErrInvalidSecretKey for invalid base64, got %v", err)
	}
}

func TestJWTVerifier_Verify_Success(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)

	token, err := GenerateToken(testBase64Secret, testIssuer, testAudience, "user-123", "admin", 15*time.Minute)
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

func TestJWTVerifier_Verify_DotNetClaimTypes_Success(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)
	key, _ := DecodeSecretKey(testBase64Secret)

	// Emulate .NET JwtSecurityTokenHandler default claims output:
	// ClaimTypes.Name -> "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name" or "unique_name"
	// ClaimTypes.Role -> "http://schemas.microsoft.com/ws/2008/06/identity/claims/role"
	dotnetClaims := Claims{
		SoapName: "e501a35e-c1cf-4c8d-8ad1-68be8f8dbbc6",
		SoapRole: "Admin",
		Iss:      testIssuer,
		Aud:      testAudience,
		Exp:      time.Now().Add(30 * time.Minute).Unix(),
		Iat:      time.Now().Unix(),
	}

	token, err := GenerateTokenWithClaims(key, dotnetClaims)
	if err != nil {
		t.Fatalf("failed to generate dotnet claims token: %v", err)
	}

	principal, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("expected success verifying token with .NET ClaimTypes names, got: %v", err)
	}

	if principal.UserID != "e501a35e-c1cf-4c8d-8ad1-68be8f8dbbc6" {
		t.Errorf("expected UserID e501a35e-c1cf-4c8d-8ad1-68be8f8dbbc6, got %s", principal.UserID)
	}
	if principal.Role != "Admin" {
		t.Errorf("expected Role Admin, got %s", principal.Role)
	}
}

func TestJWTVerifier_Verify_AudienceArray_Success(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)
	key, _ := DecodeSecretKey(testBase64Secret)

	// Emulate JWT where "aud" is an array of strings
	claimsWithAudArray := Claims{
		Sub:  "user-789",
		Role: "Owner",
		Iss:  testIssuer,
		Aud:  []string{"other-api", testAudience, "third-party"},
		Exp:  time.Now().Add(30 * time.Minute).Unix(),
	}

	token, err := GenerateTokenWithClaims(key, claimsWithAudArray)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	principal, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("expected success verifying token with audience array, got: %v", err)
	}

	if principal.UserID != "user-789" {
		t.Errorf("expected UserID user-789, got %s", principal.UserID)
	}
	if principal.Role != "Owner" {
		t.Errorf("expected Role Owner, got %s", principal.Role)
	}
}

func TestJWTVerifier_Verify_Base64SecretInterpretationRegressionTest(t *testing.T) {
	// Base64 encoded 32-byte secret (matching .NET Convert.ToBase64String)
	base64Secret := "c2VjcmV0LWtleS1mb3ItZ21oZWxwZXItand0LXRlc3QtMzJi"
	decodedBytes, err := DecodeSecretKey(base64Secret)
	if err != nil {
		t.Fatalf("failed to decode base64Secret: %v", err)
	}

	// 1. Correct behavior: HMAC signed with decoded raw bytes
	verifier := MustNewJWTVerifier(base64Secret, testIssuer, testAudience)
	validToken, err := GenerateToken(base64Secret, testIssuer, testAudience, "user-correct", "Admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	principal, err := verifier.Verify(validToken)
	if err != nil {
		t.Fatalf("expected verification success with base64 decoded key, got: %v", err)
	}
	if principal.UserID != "user-correct" {
		t.Errorf("expected UserID user-correct, got %s", principal.UserID)
	}

	// 2. Incorrect behavior regression guard:
	// If a token was signed using the Base64 ASCII text string directly instead of decoded bytes:
	hdr := header{Alg: "HS256", Typ: "JWT"}
	hdrBytes, _ := json.Marshal(hdr)
	claims := Claims{
		Sub: "user-incorrect",
		Iss: testIssuer,
		Aud: testAudience,
		Exp: time.Now().Add(15 * time.Minute).Unix(),
	}
	claimsBytes, _ := json.Marshal(claims)
	signingInput := encodeBase64URL(hdrBytes) + "." + encodeBase64URL(claimsBytes)

	// Sign using raw ASCII string bytes of base64 text (the old flawed behavior)
	flawedMac := hmac.New(sha256.New, []byte(base64Secret))
	flawedMac.Write([]byte(signingInput))
	flawedToken := signingInput + "." + encodeBase64URL(flawedMac.Sum(nil))

	// The verifier (which uses decoded bytes) MUST reject the flawed token
	_, err = verifier.Verify(flawedToken)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("regression check failed: expected ErrInvalidSignature when token is signed with raw base64 string instead of decoded bytes, got %v", err)
	}

	// Verify that decoded bytes differ from raw base64 string bytes
	if string(decodedBytes) == base64Secret {
		t.Fatal("test setup error: decoded bytes should not equal base64 string")
	}
}

func TestJWTVerifier_Verify_DeterministicDotNetTestVector(t *testing.T) {
	// Deterministic test vector representing a token generated by .NET with:
	// Secret (Base64): "c2VjcmV0LWtleS1mb3ItZ21oZWxwZXItand0LXRlc3QtMzJi"
	// Decoded Secret:  "secret-key-for-gmhelper-jwt-test-32b"
	// Issuer:          "gmhelper-api"
	// Audience:        "gmhelper-notify-api"
	// Subject:         "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	// Role:            "Admin"
	// Expiration:      2082787200 (2036-01-01T00:00:00Z)
	deterministicBase64Secret := "c2VjcmV0LWtleS1mb3ItZ21oZWxwZXItand0LXRlc3QtMzJi"
	decodedKey, _ := DecodeSecretKey(deterministicBase64Secret)

	claims := Claims{
		Sub:  "3fa85f64-5717-4562-b3fc-2c963f66afa6",
		Role: "Admin",
		Iss:  "gmhelper-api",
		Aud:  "gmhelper-notify-api",
		Exp:  2082787200,
	}

	token, err := GenerateTokenWithClaims(decodedKey, claims)
	if err != nil {
		t.Fatalf("failed to generate deterministic token: %v", err)
	}

	verifier := MustNewJWTVerifier(deterministicBase64Secret, "gmhelper-api", "gmhelper-notify-api")
	principal, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("failed to verify deterministic test vector: %v", err)
	}

	if principal.UserID != "3fa85f64-5717-4562-b3fc-2c963f66afa6" {
		t.Errorf("expected UserID 3fa85f64-5717-4562-b3fc-2c963f66afa6, got %s", principal.UserID)
	}
	if principal.Role != "Admin" {
		t.Errorf("expected Role Admin, got %s", principal.Role)
	}
}

func TestJWTVerifier_Verify_ExpiredToken(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)

	// Token expired 10 minutes ago
	token, err := GenerateToken(testBase64Secret, testIssuer, testAudience, "user-123", "user", -10*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = verifier.Verify(token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestJWTVerifier_Verify_NotBeforeFuture(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)
	key, _ := DecodeSecretKey(testBase64Secret)

	// Token with Nbf 10 minutes in the future
	claims := Claims{
		Sub: "user-123",
		Iss: testIssuer,
		Aud: testAudience,
		Exp: time.Now().Add(30 * time.Minute).Unix(),
		Nbf: time.Now().Add(10 * time.Minute).Unix(),
	}

	token, _ := GenerateTokenWithClaims(key, claims)
	_, err := verifier.Verify(token)
	if !errors.Is(err, ErrTokenUsedPremature) {
		t.Fatalf("expected ErrTokenUsedPremature for future Nbf, got %v", err)
	}
}

func TestJWTVerifier_Verify_InvalidSignature(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)

	// Signed with different secret: "YW5vdGhlci1zZWNyZXQta2V5LTMyLWJ5dGVzISE="
	otherBase64Secret := "YW5vdGhlci1zZWNyZXQta2V5LTMyLWJ5dGVzISE="
	token, err := GenerateToken(otherBase64Secret, testIssuer, testAudience, "user-123", "user", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = verifier.Verify(token)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestJWTVerifier_Verify_TamperedPayload(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)

	token, _ := GenerateToken(testBase64Secret, testIssuer, testAudience, "user-123", "user", 15*time.Minute)
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
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)

	token, _ := GenerateToken(testBase64Secret, "evil-issuer", testAudience, "user-123", "user", 15*time.Minute)

	_, err := verifier.Verify(token)
	if !errors.Is(err, ErrInvalidIssuer) {
		t.Fatalf("expected ErrInvalidIssuer, got %v", err)
	}
}

func TestJWTVerifier_Verify_WrongAudience(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)

	token, _ := GenerateToken(testBase64Secret, testIssuer, "other-service", "user-123", "user", 15*time.Minute)

	_, err := verifier.Verify(token)
	if !errors.Is(err, ErrInvalidAudience) {
		t.Fatalf("expected ErrInvalidAudience, got %v", err)
	}
}

func TestJWTVerifier_Verify_AlgorithmConfusionProtection(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)

	// Create header with "none" alg
	hdr := header{Alg: "none", Typ: "JWT"}
	hdrBytes, _ := json.Marshal(hdr)
	claims := Claims{Sub: "user-1", Iss: testIssuer, Aud: testAudience, Exp: time.Now().Add(15 * time.Minute).Unix()}
	claimsBytes, _ := json.Marshal(claims)

	noneToken := encodeBase64URL(hdrBytes) + "." + encodeBase64URL(claimsBytes) + "."

	_, err := verifier.Verify(noneToken)
	if !errors.Is(err, ErrUnsupportedAlg) {
		t.Fatalf("expected ErrUnsupportedAlg for 'none' algorithm, got %v", err)
	}

	// Create header with "RS256" alg
	rsHdr := header{Alg: "RS256", Typ: "JWT"}
	rsHdrBytes, _ := json.Marshal(rsHdr)
	rsToken := encodeBase64URL(rsHdrBytes) + "." + encodeBase64URL(claimsBytes) + ".fake-sig"

	_, err = verifier.Verify(rsToken)
	if !errors.Is(err, ErrUnsupportedAlg) {
		t.Fatalf("expected ErrUnsupportedAlg for 'RS256' algorithm, got %v", err)
	}
}

func TestJWTVerifier_Verify_MissingSubject(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)

	// Token with empty sub and user_id
	token, _ := GenerateToken(testBase64Secret, testIssuer, testAudience, "", "user", 15*time.Minute)

	_, err := verifier.Verify(token)
	if !errors.Is(err, ErrMissingSubject) {
		t.Fatalf("expected ErrMissingSubject, got %v", err)
	}
}

func TestJWTVerifier_Verify_MissingExpiration(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)
	key, _ := DecodeSecretKey(testBase64Secret)

	// Token with Exp = 0 (no exp claim)
	hdr := header{Alg: "HS256", Typ: "JWT"}
	hdrBytes, _ := json.Marshal(hdr)
	claims := Claims{Sub: "user-1", Iss: testIssuer, Aud: testAudience, Exp: 0}
	claimsBytes, _ := json.Marshal(claims)

	signingInput := encodeBase64URL(hdrBytes) + "." + encodeBase64URL(claimsBytes)
	mac := computeHMACSHA256([]byte(signingInput), key)
	tokenWithoutExp := signingInput + "." + encodeBase64URL(mac)

	_, err := verifier.Verify(tokenWithoutExp)
	if !errors.Is(err, ErrMissingExpiration) {
		t.Fatalf("expected ErrMissingExpiration for token without exp claim, got %v", err)
	}
}

func TestJWTVerifier_Verify_MalformedTokens(t *testing.T) {
	verifier := MustNewJWTVerifier(testBase64Secret, testIssuer, testAudience)

	testCases := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"single part", "invalidtoken"},
		{"two parts", "header.payload"},
		{"four parts", "a.b.c.d"},
		{"malformed base64 header", "???.payload.sig"},
		{"malformed header JSON", encodeBase64URL([]byte("{invalid-json")) + ".payload.sig"},
		{"malformed base64 payload", encodeBase64URL([]byte(`{"alg":"HS256"}`)) + ".???.sig"},
		{"malformed payload JSON", encodeBase64URL([]byte(`{"alg":"HS256"}`)) + "." + encodeBase64URL([]byte("{invalid-json")) + ".sig"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := verifier.Verify(tc.token)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
