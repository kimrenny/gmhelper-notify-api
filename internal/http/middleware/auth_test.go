package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

const (
	mwSecret   = "middleware-secret-key-32-chars!"
	mwIssuer   = "gmhelper-api"
	mwAudience = "gmhelper-notify-api"
)

func setupAuthMiddlewareTest() (Middleware, auth.TokenVerifier) {
	log, _ := logger.NewLogger("info")
	verifier := auth.NewJWTVerifier(mwSecret, mwIssuer, mwAudience)
	mw := Authenticate(verifier, log)
	return mw, verifier
}

func TestAuthenticateMiddleware_Success(t *testing.T) {
	mw, _ := setupAuthMiddlewareTest()

	token, err := auth.GenerateToken(mwSecret, mwIssuer, mwAudience, "user-456", "service", 10*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	var capturedPrincipal *domain.Principal
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := GetPrincipal(r.Context())
		if ok {
			capturedPrincipal = p
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	mw(nextHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rec.Code)
	}
	if capturedPrincipal == nil {
		t.Fatal("expected principal in context, got nil")
	}
	if capturedPrincipal.UserID != "user-456" {
		t.Errorf("expected UserID user-456, got %s", capturedPrincipal.UserID)
	}
	if capturedPrincipal.Role != "service" {
		t.Errorf("expected Role service, got %s", capturedPrincipal.Role)
	}
}

func TestAuthenticateMiddleware_MissingOrMalformedHeader(t *testing.T) {
	mw, _ := setupAuthMiddlewareTest()
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		authHeader string
	}{
		{
			name:       "Missing Header",
			authHeader: "",
		},
		{
			name:       "Basic Auth Instead of Bearer",
			authHeader: "Basic dXNlcjpwYXNz",
		},
		{
			name:       "Empty Bearer Token",
			authHeader: "Bearer ",
		},
		{
			name:       "Invalid/Malformed JWT",
			authHeader: "Bearer invalid.jwt.token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			mw(nextHandler).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected HTTP 401 Unauthorized, got %d", rec.Code)
			}

			var errResp response.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			if errResp.Error.Code != "UNAUTHORIZED" {
				t.Errorf("expected error code UNAUTHORIZED, got %s", errResp.Error.Code)
			}
		})
	}
}
