package userclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_Validation(t *testing.T) {
	// 1. Empty URL
	_, err := NewClient("", nil)
	if err == nil {
		t.Error("expected error for empty baseURL, got nil")
	}

	// 2. Whitespace URL
	_, err = NewClient("   ", nil)
	if err == nil {
		t.Error("expected error for whitespace baseURL, got nil")
	}

	// 3. Invalid scheme
	_, err = NewClient("ftp://localhost:5000", nil)
	if err == nil {
		t.Error("expected error for non-http/https scheme, got nil")
	}

	// 4. No host
	_, err = NewClient("http://", nil)
	if err == nil {
		t.Error("expected error for baseURL without host, got nil")
	}

	// 5. Valid URL with trailing slash trimmed
	client, err := NewClient("https://api.gmhelper.com/", nil)
	if err != nil {
		t.Fatalf("expected valid client, got error: %v", err)
	}
	if client.baseURL != "https://api.gmhelper.com" {
		t.Errorf("expected trimmed baseURL 'https://api.gmhelper.com', got: %s", client.baseURL)
	}
	if client.httpClient == nil || client.httpClient.Timeout != defaultTimeout {
		t.Errorf("expected default http.Client with timeout %v, got %v", defaultTimeout, client.httpClient)
	}

	// 6. Custom http.Client preserved
	customHTTP := &http.Client{Timeout: 3 * time.Second}
	clientCustom, err := NewClient("http://localhost:5000", customHTTP)
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/internal/users/e4b216c5-eef4-4f05-b1a1-39644f19816d" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept: application/json, got %s", r.Header.Get("Accept"))
		}

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

	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	user, err := client.GetUserByID(context.Background(), "e4b216c5-eef4-4f05-b1a1-39644f19816d")
	if err != nil {
		t.Fatalf("expected GetUserByID success, got error: %v", err)
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

func TestHTTPClient_GetUserByID_EmptyID(t *testing.T) {
	client, _ := NewClient("http://localhost:5000", nil)

	_, err := client.GetUserByID(context.Background(), "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty ID, got: %v", err)
	}

	_, err = client.GetUserByID(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for whitespace ID, got: %v", err)
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

	client, _ := NewClient(server.URL, server.Client())
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

	client, _ := NewClient(server.URL, server.Client())
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

	client, _ := NewClient(server.URL, server.Client())
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

	client, _ := NewClient(server.URL, server.Client())
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

	client, _ := NewClient(server.URL, server.Client())
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

	client, _ := NewClient(server.URL, server.Client())
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

	client, _ := NewClient(server.URL, server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.GetUserByID(ctx, "user-id-1")
	if err == nil {
		t.Fatal("expected error due to cancelled context, got nil")
	}
}
