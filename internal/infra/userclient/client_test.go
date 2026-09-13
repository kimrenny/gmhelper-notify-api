package userclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
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

func TestHTTPClient_GetUserByID_Non2xxUnexpected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if !errors.Is(err, ErrUnexpected) {
		t.Fatalf("expected ErrUnexpected for 403 Forbidden, got: %v", err)
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
