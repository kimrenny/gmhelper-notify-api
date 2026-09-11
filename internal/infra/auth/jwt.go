package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken       = errors.New("invalid token")
	ErrExpiredToken       = errors.New("token is expired")
	ErrInvalidSignature   = errors.New("invalid token signature")
	ErrUnsupportedAlg     = errors.New("unsupported signing algorithm")
	ErrInvalidIssuer      = errors.New("invalid token issuer")
	ErrInvalidAudience    = errors.New("invalid token audience")
	ErrMissingSubject     = errors.New("missing subject in token claims")
	ErrMissingExpiration  = errors.New("missing expiration in token claims")
	ErrTokenUsedPremature = errors.New("token used before issued timestamp")
	ErrInvalidSecretKey   = errors.New("invalid base64 secret key")
)

type TokenVerifier interface {
	Verify(tokenString string) (*domain.Principal, error)
}

type JWTVerifier struct {
	secret    []byte
	issuer    string
	audience  string
	clockSkew time.Duration
}

// DecodeSecretKey decodes a base64-encoded secret key string into signing key bytes.
// This matches .NET's Convert.FromBase64String behavior in gmhelper-api.
func DecodeSecretKey(base64Secret string) ([]byte, error) {
	trimmed := strings.TrimSpace(base64Secret)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: secret key cannot be empty", ErrInvalidSecretKey)
	}

	// Try standard base64 decoding (matching .NET Convert.FromBase64String)
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err == nil {
		if len(decoded) == 0 {
			return nil, fmt.Errorf("%w: decoded secret key is empty", ErrInvalidSecretKey)
		}
		return decoded, nil
	}

	// Try unpadded standard base64 decoding
	if unpadded, unpaddedErr := base64.RawStdEncoding.DecodeString(trimmed); unpaddedErr == nil {
		if len(unpadded) == 0 {
			return nil, fmt.Errorf("%w: decoded secret key is empty", ErrInvalidSecretKey)
		}
		return unpadded, nil
	}

	return nil, fmt.Errorf("%w: %v", ErrInvalidSecretKey, err)
}

// NewJWTVerifier constructs a JWTVerifier by decoding the base64-encoded secret key.
func NewJWTVerifier(base64Secret, issuer, audience string) (*JWTVerifier, error) {
	key, err := DecodeSecretKey(base64Secret)
	if err != nil {
		return nil, err
	}
	return NewJWTVerifierWithKey(key, issuer, audience), nil
}

// MustNewJWTVerifier constructs a JWTVerifier or panics if base64 decoding fails (useful for tests).
func MustNewJWTVerifier(base64Secret, issuer, audience string) *JWTVerifier {
	verifier, err := NewJWTVerifier(base64Secret, issuer, audience)
	if err != nil {
		panic(fmt.Sprintf("MustNewJWTVerifier failed: %v", err))
	}
	return verifier
}

// NewJWTVerifierWithKey constructs a JWTVerifier with pre-decoded raw key bytes.
func NewJWTVerifierWithKey(key []byte, issuer, audience string) *JWTVerifier {
	return &JWTVerifier{
		secret:    key,
		issuer:    issuer,
		audience:  audience,
		clockSkew: 1 * time.Minute,
	}
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// Claims represents JWT payload claims supporting both standard JWT and .NET ClaimTypes mappings.
type Claims struct {
	Sub        string `json:"sub,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	UniqueName string `json:"unique_name,omitempty"`
	SoapName   string `json:"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name,omitempty"`
	Name       string `json:"name,omitempty"`
	Role       string `json:"role,omitempty"`
	SoapRole   string `json:"http://schemas.microsoft.com/ws/2008/06/identity/claims/role,omitempty"`
	Iss        string `json:"iss,omitempty"`
	Aud        any    `json:"aud,omitempty"`
	Exp        int64  `json:"exp,omitempty"`
	Nbf        int64  `json:"nbf,omitempty"`
	Iat        int64  `json:"iat,omitempty"`
	Jti        string `json:"jti,omitempty"`
}

// GetUserID extracts the user identifier from claims across standard and .NET formats.
func (c *Claims) GetUserID() string {
	if c.Sub != "" {
		return c.Sub
	}
	if c.UserID != "" {
		return c.UserID
	}
	if c.UniqueName != "" {
		return c.UniqueName
	}
	if c.SoapName != "" {
		return c.SoapName
	}
	if c.Name != "" {
		return c.Name
	}
	return ""
}

// GetRole extracts the role claim across standard and .NET formats.
func (c *Claims) GetRole() string {
	if c.Role != "" {
		return c.Role
	}
	if c.SoapRole != "" {
		return c.SoapRole
	}
	return ""
}

// MatchesAudience verifies if the token audience satisfies the expected audience string.
func (c *Claims) MatchesAudience(expected string) bool {
	if expected == "" {
		return true
	}
	if c.Aud == nil {
		return false
	}
	switch v := c.Aud.(type) {
	case string:
		return v == expected
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == expected {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if s == expected {
				return true
			}
		}
	}
	return false
}

// Verify validates and cryptographically verifies a JWT token.
func (v *JWTVerifier) Verify(tokenString string) (*domain.Principal, error) {
	if len(v.secret) == 0 {
		return nil, errors.New("auth secret is not configured")
	}

	tokenString = strings.TrimSpace(tokenString)
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	headerBytes, err := decodeBase64URL(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: malformed header", ErrInvalidToken)
	}

	var hdr header
	if err := json.Unmarshal(headerBytes, &hdr); err != nil {
		return nil, fmt.Errorf("%w: malformed header JSON", ErrInvalidToken)
	}

	// Strictly require HS256. Disallow 'none', 'RS256', etc.
	if hdr.Alg != "HS256" {
		return nil, fmt.Errorf("%w: %s (expected HS256)", ErrUnsupportedAlg, hdr.Alg)
	}

	// Verify cryptographic signature
	signingInput := parts[0] + "." + parts[1]
	expectedMAC := computeHMACSHA256([]byte(signingInput), v.secret)

	actualSig, err := decodeBase64URL(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: malformed signature", ErrInvalidSignature)
	}

	if !hmac.Equal(expectedMAC, actualSig) {
		return nil, ErrInvalidSignature
	}

	// Parse payload claims
	payloadBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: malformed payload", ErrInvalidToken)
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("%w: malformed payload JSON", ErrInvalidToken)
	}

	// Validate Issuer
	if v.issuer != "" && claims.Iss != v.issuer {
		return nil, fmt.Errorf("%w: got %s, expected %s", ErrInvalidIssuer, claims.Iss, v.issuer)
	}

	// Validate Audience
	if v.audience != "" && !claims.MatchesAudience(v.audience) {
		return nil, fmt.Errorf("%w: token audience does not match %s", ErrInvalidAudience, v.audience)
	}

	now := time.Now().UTC().Unix()

	// Validate Expiration (mandatory)
	if claims.Exp == 0 {
		return nil, ErrMissingExpiration
	}
	if now > claims.Exp {
		return nil, ErrExpiredToken
	}

	// Validate Not Before (nbf)
	if claims.Nbf != 0 && claims.Nbf > (now+int64(v.clockSkew.Seconds())) {
		return nil, ErrTokenUsedPremature
	}

	// Validate Issued At (iat)
	if claims.Iat != 0 && claims.Iat > (now+int64(v.clockSkew.Seconds())) {
		return nil, ErrTokenUsedPremature
	}

	// Extract Principal User ID
	userID := claims.GetUserID()
	if userID == "" {
		return nil, ErrMissingSubject
	}

	return &domain.Principal{
		UserID: userID,
		Role:   claims.GetRole(),
	}, nil
}

// GenerateToken creates a signed HS256 JWT using a base64-encoded secret key.
func GenerateToken(base64Secret, issuer, audience, userID, role string, ttl time.Duration) (string, error) {
	key, err := DecodeSecretKey(base64Secret)
	if err != nil {
		return "", err
	}
	return GenerateTokenWithKey(key, issuer, audience, userID, role, ttl)
}

// GenerateTokenWithKey creates a signed HS256 JWT using raw secret key bytes.
func GenerateTokenWithKey(key []byte, issuer, audience, userID, role string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		Sub:    userID,
		UserID: userID,
		Role:   role,
		Iss:    issuer,
		Aud:    audience,
		Exp:    now.Add(ttl).Unix(),
		Iat:    now.Unix(),
		Jti:    uuid.NewString(),
	}
	return GenerateTokenWithClaims(key, claims)
}

// GenerateTokenWithClaims creates a signed HS256 JWT using raw key bytes and explicit Claims.
func GenerateTokenWithClaims(key []byte, claims Claims) (string, error) {
	hdr := header{
		Alg: "HS256",
		Typ: "JWT",
	}

	hdrBytes, err := json.Marshal(hdr)
	if err != nil {
		return "", err
	}

	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encodedHdr := encodeBase64URL(hdrBytes)
	encodedPayload := encodeBase64URL(claimsBytes)
	signingInput := encodedHdr + "." + encodedPayload

	mac := computeHMACSHA256([]byte(signingInput), key)
	encodedSig := encodeBase64URL(mac)

	return signingInput + "." + encodedSig, nil
}

func computeHMACSHA256(data, secret []byte) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write(data)
	return h.Sum(nil)
}

func encodeBase64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// EncodeBase64ForTest exports unpadded base64url encoding for test assertion generation.
func EncodeBase64ForTest(data []byte) string {
	return encodeBase64URL(data)
}

func decodeBase64URL(s string) ([]byte, error) {
	// Support both unpadded and padded base64url encodings
	if l := len(s) % 4; l > 0 {
		s += strings.Repeat("=", 4-l)
	}
	return base64.URLEncoding.DecodeString(s)
}
