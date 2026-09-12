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
	mwSecret   = "bWlkZGxld2FyZS1zZWNyZXQta2V5LTMyLWNoYXJzISE="
	mwIssuer   = "gmhelper-api"
	mwAudience = "gmhelper-notify-api"
)

func setupAuthMiddlewareTest() (Middleware, auth.TokenVerifier) {
	log, _ := logger.NewLogger("info")
	verifier := auth.MustNewJWTVerifier(mwSecret, mwIssuer, mwAudience)
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

func TestRequireRole_Scenarios(t *testing.T) {
	tests := []struct {
		name         string
		principal    *domain.Principal
		allowedRoles []string
		wantStatus   int
		wantCode     string
	}{
		{
			name:         "Allowed role matched exact",
			principal:    &domain.Principal{UserID: "u1", Role: "admin"},
			allowedRoles: []string{"admin", "owner"},
			wantStatus:   http.StatusOK,
		},
		{
			name:         "Allowed role matched case-insensitively",
			principal:    &domain.Principal{UserID: "u2", Role: "ADMIN"},
			allowedRoles: []string{"admin", "owner"},
			wantStatus:   http.StatusOK,
		},
		{
			name:         "Allowed role matched second item",
			principal:    &domain.Principal{UserID: "u3", Role: "service"},
			allowedRoles: []string{"admin", "owner", "service"},
			wantStatus:   http.StatusOK,
		},
		{
			name:         "Disallowed role returns 403 Forbidden",
			principal:    &domain.Principal{UserID: "u4", Role: "user"},
			allowedRoles: []string{"admin", "owner", "service"},
			wantStatus:   http.StatusForbidden,
			wantCode:     "FORBIDDEN",
		},
		{
			name:         "Empty role on principal returns 403 Forbidden",
			principal:    &domain.Principal{UserID: "u5", Role: ""},
			allowedRoles: []string{"admin", "owner"},
			wantStatus:   http.StatusForbidden,
			wantCode:     "FORBIDDEN",
		},
		{
			name:         "No principal in context returns 401 Unauthorized",
			principal:    nil,
			allowedRoles: []string{"admin"},
			wantStatus:   http.StatusUnauthorized,
			wantCode:     "UNAUTHORIZED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := RequireRole(tt.allowedRoles...)
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.principal != nil {
				req = req.WithContext(ContextWithPrincipal(req.Context(), tt.principal))
			}
			rec := httptest.NewRecorder()

			mw(nextHandler).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			if tt.wantCode != "" {
				var errResp response.ErrorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if errResp.Error.Code != tt.wantCode {
					t.Errorf("expected error code %s, got %s", tt.wantCode, errResp.Error.Code)
				}
			}
		})
	}
}

func TestAdminAuth_EndToEndMatrix(t *testing.T) {
	log, _ := logger.NewLogger("info")
	verifier := auth.MustNewJWTVerifier(mwSecret, mwIssuer, mwAudience)
	adminMw := AdminAuth(verifier, log)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	tests := []struct {
		name       string
		tokenGen   func() string
		authHeader string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "Missing header -> 401",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "Malformed token -> 401",
			authHeader: "Bearer invalid.token.value",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name: "Expired admin token -> 401",
			tokenGen: func() string {
				tok, _ := auth.GenerateToken(mwSecret, mwIssuer, mwAudience, "u-admin", "admin", -5*time.Minute)
				return "Bearer " + tok
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name: "Invalid signature admin token -> 401",
			tokenGen: func() string {
				tok, _ := auth.GenerateToken("wrong-secret-key-32-characters!", mwIssuer, mwAudience, "u-admin", "admin", 15*time.Minute)
				return "Bearer " + tok
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name: "Valid user role token -> 403 Forbidden",
			tokenGen: func() string {
				tok, _ := auth.GenerateToken(mwSecret, mwIssuer, mwAudience, "u-regular", "user", 15*time.Minute)
				return "Bearer " + tok
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name: "Valid guest role token -> 403 Forbidden",
			tokenGen: func() string {
				tok, _ := auth.GenerateToken(mwSecret, mwIssuer, mwAudience, "u-guest", "guest", 15*time.Minute)
				return "Bearer " + tok
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name: "Valid admin role token -> 200 OK",
			tokenGen: func() string {
				tok, _ := auth.GenerateToken(mwSecret, mwIssuer, mwAudience, "u-admin", "admin", 15*time.Minute)
				return "Bearer " + tok
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "Valid owner role token -> 200 OK",
			tokenGen: func() string {
				tok, _ := auth.GenerateToken(mwSecret, mwIssuer, mwAudience, "u-owner", "owner", 15*time.Minute)
				return "Bearer " + tok
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "Valid service role token -> 200 OK",
			tokenGen: func() string {
				tok, _ := auth.GenerateToken(mwSecret, mwIssuer, mwAudience, "u-service", "service", 15*time.Minute)
				return "Bearer " + tok
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := tt.authHeader
			if tt.tokenGen != nil {
				header = tt.tokenGen()
			}

			req := httptest.NewRequest(http.MethodPost, "/admin/operation", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()

			adminMw(nextHandler).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			if tt.wantCode != "" {
				var errResp response.ErrorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if errResp.Error.Code != tt.wantCode {
					t.Errorf("expected error code %s, got %s", tt.wantCode, errResp.Error.Code)
				}
			}
		})
	}
}
