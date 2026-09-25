package userclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/infra/auth"
)

type fakeTokenProvider struct {
	tokenFunc func(ctx context.Context) (string, error)
}

func (f *fakeTokenProvider) Token(ctx context.Context) (string, error) {
	if f.tokenFunc != nil {
		return f.tokenFunc(ctx)
	}
	return "test-service-token", nil
}

func TestNewClient_Validation(t *testing.T) {
	fakeTP := &fakeTokenProvider{}

	// 1. Empty URL
	_, err := NewClient("", nil, fakeTP)
	if err == nil {
		t.Error("expected error for empty baseURL, got nil")
	}

	// 2. Whitespace URL
	_, err = NewClient("   ", nil, fakeTP)
	if err == nil {
		t.Error("expected error for whitespace baseURL, got nil")
	}

	// 3. Invalid scheme
	_, err = NewClient("ftp://localhost:5000", nil, fakeTP)
	if err == nil {
		t.Error("expected error for non-http/https scheme, got nil")
	}

	// 4. No host
	_, err = NewClient("http://", nil, fakeTP)
	if err == nil {
		t.Error("expected error for baseURL without host, got nil")
	}

	// 5. Nil token provider rejected
	_, err = NewClient("https://api.gmhelper.com", nil, nil)
	if !errors.Is(err, ErrMissingTokenProvider) {
		t.Errorf("expected ErrMissingTokenProvider for nil token provider, got: %v", err)
	}

	// 6. Valid URL with trailing slash trimmed
	client, err := NewClient("https://api.gmhelper.com/", nil, fakeTP)
	if err != nil {
		t.Fatalf("expected valid client, got error: %v", err)
	}
	if client.baseURL != "https://api.gmhelper.com" {
		t.Errorf("expected trimmed baseURL 'https://api.gmhelper.com', got: %s", client.baseURL)
	}
	if client.httpClient == nil || client.httpClient.Timeout != defaultTimeout {
		t.Errorf("expected default http.Client with timeout %v, got %v", defaultTimeout, client.httpClient)
	}

	// 7. Custom http.Client preserved
	customHTTP := &http.Client{Timeout: 3 * time.Second}
	clientCustom, err := NewClient("http://localhost:5000", customHTTP, fakeTP)
	if err != nil {
		t.Fatalf("expected valid client, got error: %v", err)
	}
	if clientCustom.httpClient != customHTTP {
		t.Errorf("expected custom http.Client to be preserved")
	}
}

func TestHTTPClient_GetUserByID_Success(t *testing.T) {
	regDate := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	expectedUser := User{
		ID:               "e4b216c5-eef4-4f05-b1a1-39644f19816d",
		Username:         "john_doe",
		Email:            "john@example.com",
		Role:             "Admin",
		Language:         "en",
		IsActive:         true,
		IsBlocked:        false,
		RegistrationDate: regDate,
	}

	var capturedAuthHeader string
	var capturedAcceptHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/internal/users/e4b216c5-eef4-4f05-b1a1-39644f19816d" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}

		capturedAuthHeader = r.Header.Get("Authorization")
		capturedAcceptHeader = r.Header.Get("Accept")

		resp := apiResponse[User]{
			Success: true,
			Message: nil,
			Data:    &expectedUser,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	var receivedContext context.Context
	fakeTP := &fakeTokenProvider{
		tokenFunc: func(ctx context.Context) (string, error) {
			receivedContext = ctx
			return "valid-service-jwt-token", nil
		},
	}

	client, err := NewClient(server.URL, server.Client(), fakeTP)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	reqCtx := context.WithValue(context.Background(), "trace-key", "trace-val")
	user, err := client.GetUserByID(reqCtx, "e4b216c5-eef4-4f05-b1a1-39644f19816d")
	if err != nil {
		t.Fatalf("expected GetUserByID success, got error: %v", err)
	}

	if capturedAuthHeader != "Bearer valid-service-jwt-token" {
		t.Errorf("expected Authorization 'Bearer valid-service-jwt-token', got '%s'", capturedAuthHeader)
	}
	if capturedAcceptHeader != "application/json" {
		t.Errorf("expected Accept 'application/json', got '%s'", capturedAcceptHeader)
	}
	if receivedContext == nil || receivedContext.Value("trace-key") != "trace-val" {
		t.Errorf("expected token provider to receive request context with trace-key")
	}

	if user.ID != expectedUser.ID {
		t.Errorf("expected user ID %s, got %s", expectedUser.ID, user.ID)
	}
	if user.Username != expectedUser.Username {
		t.Errorf("expected username %s, got %s", expectedUser.Username, user.Username)
	}
	if user.Email != expectedUser.Email {
		t.Errorf("expected email %s, got %s", expectedUser.Email, user.Email)
	}
	if user.Role != expectedUser.Role {
		t.Errorf("expected role %s, got %s", expectedUser.Role, user.Role)
	}
	if user.Language != expectedUser.Language {
		t.Errorf("expected language %s, got %s", expectedUser.Language, user.Language)
	}
	if !user.IsActive {
		t.Errorf("expected IsActive true, got %v", user.IsActive)
	}
	if user.IsBlocked {
		t.Errorf("expected IsBlocked false, got %v", user.IsBlocked)
	}
	if !user.RegistrationDate.Equal(expectedUser.RegistrationDate) {
		t.Errorf("expected RegistrationDate %v, got %v", expectedUser.RegistrationDate, user.RegistrationDate)
	}
}

func TestHTTPClient_GetUserByID_TokenProviderError(t *testing.T) {
	var httpRequestsCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpRequestsCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fakeTP := &fakeTokenProvider{
		tokenFunc: func(ctx context.Context) (string, error) {
			return "", errors.New("signing key unavailable")
		},
	}

	client, err := NewClient(server.URL, server.Client(), fakeTP)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.GetUserByID(context.Background(), "user-id-1")
	if err == nil {
		t.Fatal("expected error when token provider fails, got nil")
	}
	if !errors.Is(err, ErrTokenAcquisition) {
		t.Errorf("expected ErrTokenAcquisition, got: %v", err)
	}

	if count := atomic.LoadInt32(&httpRequestsCount); count != 0 {
		t.Errorf("expected 0 HTTP requests when token provider fails, got %d", count)
	}
}

func TestHTTPClient_GetUserByID_EmptyTokenFromProvider(t *testing.T) {
	var httpRequestsCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpRequestsCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fakeTP := &fakeTokenProvider{
		tokenFunc: func(ctx context.Context) (string, error) {
			return "   ", nil
		},
	}

	client, err := NewClient(server.URL, server.Client(), fakeTP)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.GetUserByID(context.Background(), "user-id-1")
	if !errors.Is(err, ErrTokenAcquisition) {
		t.Errorf("expected ErrTokenAcquisition for empty token, got: %v", err)
	}
	if count := atomic.LoadInt32(&httpRequestsCount); count != 0 {
		t.Errorf("expected 0 HTTP requests, got %d", count)
	}
}

func TestHTTPClient_GetUserByID_DynamicTokenProvider(t *testing.T) {
	var capturedTokens []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTokens = append(capturedTokens, r.Header.Get("Authorization"))
		resp := apiResponse[User]{
			Success: true,
			Data: &User{
				ID:       "user-1",
				Username: "user1",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	var counter int
	fakeTP := &fakeTokenProvider{
		tokenFunc: func(ctx context.Context) (string, error) {
			counter++
			if counter == 1 {
				return "jwt-token-alpha", nil
			}
			return "jwt-token-beta", nil
		},
	}

	client, err := NewClient(server.URL, server.Client(), fakeTP)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.GetUserByID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("call 1 failed: %v", err)
	}

	_, err = client.GetUserByID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("call 2 failed: %v", err)
	}

	if len(capturedTokens) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(capturedTokens))
	}
	if capturedTokens[0] != "Bearer jwt-token-alpha" {
		t.Errorf("expected first call 'Bearer jwt-token-alpha', got: %s", capturedTokens[0])
	}
	if capturedTokens[1] != "Bearer jwt-token-beta" {
		t.Errorf("expected second call 'Bearer jwt-token-beta', got: %s", capturedTokens[1])
	}
}

func TestHTTPClient_GetUserByID_EmptyID(t *testing.T) {
	var tokenRequested bool
	fakeTP := &fakeTokenProvider{
		tokenFunc: func(ctx context.Context) (string, error) {
			tokenRequested = true
			return "token", nil
		},
	}
	client, _ := NewClient("http://localhost:5000", nil, fakeTP)

	_, err := client.GetUserByID(context.Background(), "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty ID, got: %v", err)
	}

	_, err = client.GetUserByID(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for whitespace ID, got: %v", err)
	}

	if tokenRequested {
		t.Error("expected empty user ID validation to fail before token acquisition")
	}
}

func TestHTTPClient_GetUserByID_NotFound(t *testing.T) {
	msg := "User not found."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": msg,
			"data":    nil,
		})
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	_, err := client.GetUserByID(context.Background(), "non-existent-user-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestHTTPClient_GetUserByID_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Internal server error.",
			"data":    nil,
		})
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	_, err := client.GetUserByID(context.Background(), "user-id-1")
	if !errors.Is(err, ErrServer) {
		t.Fatalf("expected ErrServer, got: %v", err)
	}
}

func TestHTTPClient_GetUserByID_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Authorization header is missing or invalid",
			"data":    nil,
		})
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	_, err := client.GetUserByID(context.Background(), "user-id-1")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for 401, got: %v", err)
	}
}

func TestHTTPClient_GetUserByID_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Forbidden access.",
			"data":    nil,
		})
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	_, err := client.GetUserByID(context.Background(), "user-id-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for 403 Forbidden, got: %v", err)
	}
}

func TestHTTPClient_GetUserByID_Non2xxUnexpected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "I'm a teapot",
			"data":    nil,
		})
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	_, err := client.GetUserByID(context.Background(), "user-id-1")
	if !errors.Is(err, ErrUnexpected) {
		t.Fatalf("expected ErrUnexpected for 418 Teapot, got: %v", err)
	}
}

func TestHTTPClient_GetUserByID_NullableLastActivityAt(t *testing.T) {
	// Case 1: lastActivityAt is populated
	activityTime := time.Date(2026, 9, 20, 14, 30, 0, 0, time.UTC)
	serverWithActivity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":               "u-1",
				"username":         "alice",
				"email":            "alice@example.com",
				"role":             "User",
				"language":         "EN",
				"isActive":         true,
				"isBlocked":        false,
				"registrationDate": "2026-01-01T00:00:00Z",
				"lastActivityAt":   "2026-09-20T14:30:00Z",
			},
		})
	}))
	defer serverWithActivity.Close()

	client1, _ := NewClient(serverWithActivity.URL, serverWithActivity.Client(), &fakeTokenProvider{})
	userWith, err := client1.GetUserByID(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userWith.LastActivityAt == nil {
		t.Fatal("expected non-nil LastActivityAt")
	}
	if !userWith.LastActivityAt.Equal(activityTime) {
		t.Errorf("expected %v, got %v", activityTime, *userWith.LastActivityAt)
	}

	// Case 2: lastActivityAt is null
	serverWithoutActivity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":               "u-2",
				"username":         "bob",
				"email":            "bob@example.com",
				"role":             "User",
				"language":         "RU",
				"isActive":         true,
				"isBlocked":        false,
				"registrationDate": "2026-01-01T00:00:00Z",
				"lastActivityAt":   nil,
			},
		})
	}))
	defer serverWithoutActivity.Close()

	client2, _ := NewClient(serverWithoutActivity.URL, serverWithoutActivity.Client(), &fakeTokenProvider{})
	userWithout, err := client2.GetUserByID(context.Background(), "u-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userWithout.LastActivityAt != nil {
		t.Errorf("expected nil LastActivityAt, got %v", userWithout.LastActivityAt)
	}
}

func TestHTTPClient_GetUserByID_WithRealJWTServiceTokenProvider(t *testing.T) {
	secret := "Z21oZWxwZXItZGVmYXVsdC1qd3Qtc2VjcmV0LTMyYiE="
	issuer := "gmhelper-api"
	audience := "gmhelper-notify-api"

	tp, err := auth.NewServiceTokenProvider(auth.ServiceTokenProviderConfig{
		Secret:   secret,
		Issuer:   issuer,
		Audience: audience,
		TTL:      10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("failed to create real token provider: %v", err)
	}

	verifier, err := auth.NewJWTVerifier(secret, issuer, audience)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}

	var verifiedRole string
	var verifiedSub string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHdr := r.Header.Get("Authorization")
		tokenStr := strings.TrimPrefix(authHdr, "Bearer ")

		claims, err := verifier.Verify(tokenStr)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": err.Error()})
			return
		}

		verifiedRole = claims.Role
		verifiedSub = claims.UserID

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":               "u-real",
				"username":         "service_user",
				"email":            "service@example.com",
				"role":             "Service",
				"language":         "EN",
				"isActive":         true,
				"isBlocked":        false,
				"registrationDate": "2026-01-01T00:00:00Z",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client(), tp)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	u, err := client.GetUserByID(context.Background(), "u-real")
	if err != nil {
		t.Fatalf("GetUserByID with real token provider failed: %v", err)
	}

	if u.ID != "u-real" {
		t.Errorf("expected user ID u-real, got %s", u.ID)
	}
	if verifiedRole != "Service" {
		t.Errorf("expected verified token role 'Service', got %s", verifiedRole)
	}
	if verifiedSub != "gmhelper-api" {
		t.Errorf("expected verified token sub 'gmhelper-api', got %s", verifiedSub)
	}
}

func TestHTTPClient_GetUserByID_TransportErrors(t *testing.T) {
	// 1. Connection refused / closed server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close() // Close immediately

	client, _ := NewClient(serverURL, &http.Client{Timeout: 500 * time.Millisecond}, &fakeTokenProvider{})
	_, err := client.GetUserByID(context.Background(), "u-1")
	if err == nil {
		t.Fatal("expected transport error for closed server, got nil")
	}

	// 2. Timeout
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	timeoutClient, _ := NewClient(slowServer.URL, &http.Client{Timeout: 20 * time.Millisecond}, &fakeTokenProvider{})
	_, err = timeoutClient.GetUserByID(context.Background(), "u-1")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestHTTPClient_GetUserByID_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{invalid-json-payload"))
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	_, err := client.GetUserByID(context.Background(), "user-id-1")
	if !errors.Is(err, ErrUnexpected) {
		t.Fatalf("expected ErrUnexpected for invalid JSON, got: %v", err)
	}
}

func TestHTTPClient_GetUserByID_UnsuccessfulAPIResponse(t *testing.T) {
	msg := "Account suspended"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": msg,
			"data":    nil,
		})
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	_, err := client.GetUserByID(context.Background(), "user-id-1")
	if !errors.Is(err, ErrAPIUnsuccessful) {
		t.Fatalf("expected ErrAPIUnsuccessful, got: %v", err)
	}
}

func TestHTTPClient_GetUserByID_NilData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": nil,
			"data":    nil,
		})
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	_, err := client.GetUserByID(context.Background(), "user-id-1")
	if !errors.Is(err, ErrUnexpected) {
		t.Fatalf("expected ErrUnexpected when data is nil, got: %v", err)
	}
}

func TestHTTPClient_GetUserByID_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.GetUserByID(ctx, "user-id-1")
	if err == nil {
		t.Fatal("expected error due to cancelled context, got nil")
	}
}

func TestHTTPClient_SearchUsers_Success_MultipleUsers(t *testing.T) {
	regDate := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	expectedUsers := []User{
		{
			ID:               "11111111-1111-1111-1111-111111111111",
			Username:         "alice",
			Email:            "alice@example.com",
			Role:             "Admin",
			Language:         "EN",
			IsActive:         true,
			IsBlocked:        false,
			RegistrationDate: regDate,
		},
		{
			ID:               "22222222-2222-2222-2222-222222222222",
			Username:         "bob",
			Email:            "bob@example.com",
			Role:             "User",
			Language:         "RU",
			IsActive:         true,
			IsBlocked:        false,
			RegistrationDate: regDate,
		},
	}

	var capturedAuthHeader string
	var capturedAcceptHeader string
	var capturedQueryParam string
	var capturedLimitParam string
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		capturedPath = r.URL.Path
		capturedAuthHeader = r.Header.Get("Authorization")
		capturedAcceptHeader = r.Header.Get("Accept")
		capturedQueryParam = r.URL.Query().Get("query")
		capturedLimitParam = r.URL.Query().Get("limit")

		resp := apiResponse[[]User]{
			Success: true,
			Message: nil,
			Data:    &expectedUsers,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fakeTP := &fakeTokenProvider{
		tokenFunc: func(ctx context.Context) (string, error) {
			return "search-service-jwt-token", nil
		},
	}

	client, err := NewClient(server.URL, server.Client(), fakeTP)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	users, err := client.SearchUsers(context.Background(), "al & bob+test@example.com", 25)
	if err != nil {
		t.Fatalf("expected SearchUsers success, got error: %v", err)
	}

	if capturedPath != "/api/v1/internal/users/search" {
		t.Errorf("expected path '/api/v1/internal/users/search', got '%s'", capturedPath)
	}
	if capturedAuthHeader != "Bearer search-service-jwt-token" {
		t.Errorf("expected Authorization 'Bearer search-service-jwt-token', got '%s'", capturedAuthHeader)
	}
	if capturedAcceptHeader != "application/json" {
		t.Errorf("expected Accept 'application/json', got '%s'", capturedAcceptHeader)
	}
	if capturedQueryParam != "al & bob+test@example.com" {
		t.Errorf("expected decoded query 'al & bob+test@example.com', got '%s'", capturedQueryParam)
	}
	if capturedLimitParam != "25" {
		t.Errorf("expected limit '25', got '%s'", capturedLimitParam)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Username != "alice" || users[1].Username != "bob" {
		t.Errorf("unexpected users returned: %+v", users)
	}
}

func TestHTTPClient_SearchUsers_EmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		emptyList := []User{}
		resp := apiResponse[[]User]{
			Success: true,
			Data:    &emptyList,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	users, err := client.SearchUsers(context.Background(), "unknown", 20)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected empty user list, got %d items", len(users))
	}
}

func TestHTTPClient_SearchUsers_Validation_EmptyQuery(t *testing.T) {
	var tokenRequested bool
	fakeTP := &fakeTokenProvider{
		tokenFunc: func(ctx context.Context) (string, error) {
			tokenRequested = true
			return "token", nil
		},
	}
	client, _ := NewClient("http://localhost:5000", nil, fakeTP)

	// Empty query
	_, err := client.SearchUsers(context.Background(), "", 20)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty query, got: %v", err)
	}

	// Whitespace query
	_, err = client.SearchUsers(context.Background(), "   \t   ", 20)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for whitespace query, got: %v", err)
	}

	if tokenRequested {
		t.Error("expected empty query validation to fail before token acquisition")
	}
}

func TestHTTPClient_SearchUsers_TokenAcquisitionFailure(t *testing.T) {
	fakeTP := &fakeTokenProvider{
		tokenFunc: func(ctx context.Context) (string, error) {
			return "", errors.New("token secret corrupted")
		},
	}

	client, _ := NewClient("http://localhost:5000", nil, fakeTP)
	_, err := client.SearchUsers(context.Background(), "test", 20)
	if !errors.Is(err, ErrTokenAcquisition) {
		t.Errorf("expected ErrTokenAcquisition, got: %v", err)
	}
}

func TestHTTPClient_SearchUsers_EmptyTokenFailure(t *testing.T) {
	fakeTP := &fakeTokenProvider{
		tokenFunc: func(ctx context.Context) (string, error) {
			return "   ", nil
		},
	}

	client, _ := NewClient("http://localhost:5000", nil, fakeTP)
	_, err := client.SearchUsers(context.Background(), "test", 20)
	if !errors.Is(err, ErrTokenAcquisition) {
		t.Errorf("expected ErrTokenAcquisition for empty token, got: %v", err)
	}
}

func TestHTTPClient_SearchUsers_UpstreamErrors(t *testing.T) {
	// 1. 500 Server Error
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Database query failed",
		})
	}))
	defer server500.Close()

	client500, _ := NewClient(server500.URL, server500.Client(), &fakeTokenProvider{})
	_, err := client500.SearchUsers(context.Background(), "alice", 20)
	if !errors.Is(err, ErrServer) {
		t.Errorf("expected ErrServer for 500, got: %v", err)
	}

	// 2. 403 Forbidden
	server403 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server403.Close()

	client403, _ := NewClient(server403.URL, server403.Client(), &fakeTokenProvider{})
	_, err = client403.SearchUsers(context.Background(), "alice", 20)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden for 403, got: %v", err)
	}

	// 2b. 401 Unauthorized
	server401 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server401.Close()

	client401, _ := NewClient(server401.URL, server401.Client(), &fakeTokenProvider{})
	_, err = client401.SearchUsers(context.Background(), "alice", 20)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized for 401, got: %v", err)
	}

	// 3. Malformed JSON
	serverBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer serverBadJSON.Close()

	clientBadJSON, _ := NewClient(serverBadJSON.URL, serverBadJSON.Client(), &fakeTokenProvider{})
	_, err = clientBadJSON.SearchUsers(context.Background(), "alice", 20)
	if !errors.Is(err, ErrUnexpected) {
		t.Errorf("expected ErrUnexpected for bad JSON, got: %v", err)
	}

	// 4. API Unsuccessful (success=false)
	serverUnsuccessful := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Search disabled",
		})
	}))
	defer serverUnsuccessful.Close()

	clientUnsuccessful, _ := NewClient(serverUnsuccessful.URL, serverUnsuccessful.Client(), &fakeTokenProvider{})
	_, err = clientUnsuccessful.SearchUsers(context.Background(), "alice", 20)
	if !errors.Is(err, ErrAPIUnsuccessful) {
		t.Errorf("expected ErrAPIUnsuccessful, got: %v", err)
	}

	// 5. Nil Data
	serverNilData := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    nil,
		})
	}))
	defer serverNilData.Close()

	clientNilData, _ := NewClient(serverNilData.URL, serverNilData.Client(), &fakeTokenProvider{})
	_, err = clientNilData.SearchUsers(context.Background(), "alice", 20)
	if !errors.Is(err, ErrUnexpected) {
		t.Errorf("expected ErrUnexpected for nil data, got: %v", err)
	}
}

func TestHTTPClient_SearchUsers_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.SearchUsers(ctx, "test", 20)
	if err == nil {
		t.Fatal("expected error due to cancelled context, got nil")
	}
}

func TestHTTPClient_GetUsers_Success(t *testing.T) {
	regDate := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	expectedPaged := PagedUsers{
		Items: []User{
			{
				ID:               "u-1",
				Username:         "alice",
				Email:            "alice@example.com",
				Role:             "User",
				Language:         "en",
				IsActive:         true,
				IsBlocked:        false,
				RegistrationDate: regDate,
			},
		},
		TotalCount:  1,
		Page:        1,
		PageSize:    250,
		HasNextPage: false,
	}

	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/internal/users" {
			t.Errorf("expected path /api/v1/internal/users, got %s", r.URL.Path)
		}
		capturedQuery = r.URL.RawQuery

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    expectedPaged,
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.Client(), &fakeTokenProvider{})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	result, err := client.GetUsers(context.Background(), 1, 250, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Username != "alice" {
		t.Errorf("expected alice, got %s", result.Items[0].Username)
	}
	if capturedQuery != "activeOnly=true&page=1&pageSize=250&unblockedOnly=true" {
		t.Errorf("unexpected query string: %s", capturedQuery)
	}
}

func TestHTTPClient_GetUsers_Validation(t *testing.T) {
	client, _ := NewClient("http://localhost:5000", nil, &fakeTokenProvider{})

	// 1. Page < 1
	_, err := client.GetUsers(context.Background(), 0, 50, true, true)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for page 0, got: %v", err)
	}

	// 2. PageSize < 1
	_, err = client.GetUsers(context.Background(), 1, 0, true, true)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for pageSize 0, got: %v", err)
	}
}

func TestHTTPClient_GetUsers_Errors(t *testing.T) {
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server500.Close()

	client, _ := NewClient(server500.URL, server500.Client(), &fakeTokenProvider{})
	_, err := client.GetUsers(context.Background(), 1, 50, true, true)
	if !errors.Is(err, ErrServer) {
		t.Errorf("expected ErrServer, got: %v", err)
	}
}
