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

func NewJWTVerifier(secret, issuer, audience string) *JWTVerifier {
	return &JWTVerifier{
		secret:    []byte(secret),
		issuer:    issuer,
		audience:  audience,
		clockSkew: 1 * time.Minute,
	}
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type Claims struct {
	Sub    string `json:"sub,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Role   string `json:"role,omitempty"`
	Iss    string `json:"iss,omitempty"`
	Aud    string `json:"aud,omitempty"`
	Exp    int64  `json:"exp,omitempty"`
	Iat    int64  `json:"iat,omitempty"`
	Jti    string `json:"jti,omitempty"`
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
	if v.audience != "" && claims.Aud != v.audience {
		return nil, fmt.Errorf("%w: got %s, expected %s", ErrInvalidAudience, claims.Aud, v.audience)
	}

	now := time.Now().UTC().Unix()

	// Validate Expiration (mandatory)
	if claims.Exp == 0 {
		return nil, ErrMissingExpiration
	}
	if now > claims.Exp {
		return nil, ErrExpiredToken
	}

	// Validate Issued At with clock skew
	if claims.Iat != 0 && claims.Iat > (now+int64(v.clockSkew.Seconds())) {
		return nil, ErrTokenUsedPremature
	}

	// Extract Principal User ID
	userID := claims.Sub
	if userID == "" {
		userID = claims.UserID
	}
	if userID == "" {
		return nil, ErrMissingSubject
	}

	return &domain.Principal{
		UserID: userID,
		Role:   claims.Role,
	}, nil
}

// GenerateToken creates a signed HS256 JWT for service-to-service testing or communication.
func GenerateToken(secret, issuer, audience, userID, role string, ttl time.Duration) (string, error) {
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

	mac := computeHMACSHA256([]byte(signingInput), []byte(secret))
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
