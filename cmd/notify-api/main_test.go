package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/user"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

const (
	testServiceSecret   = "dGVzdC1zZXJ2aWNlLXNlY3JldC1rZXktMzItYnl0ZXMhIQ=="
	testServiceIssuer   = "gmhelper-api"
	testServiceAudience = "gmhelper-api"
)

func TestCompositionRoot_UserServiceWiring_Success(t *testing.T) {
	regDate := time.Date(2026, 9, 13, 15, 0, 0, 0, time.UTC)
	expectedUser := userclient.User{
		ID:               "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
		Username:         "service_user",
		Email:            "service@example.com",
		Role:             "Service",
		Language:         "en",
		IsActive:         true,
		IsBlocked:        false,
		RegistrationDate: regDate,
	}

	var capturedAuth string
	var capturedAccept string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedAccept = r.Header.Get("Accept")

		if r.URL.Path != "/api/v1/internal/users/a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": nil,
			"data":    expectedUser,
		})
	}))
	defer server.Close()

	// 1. Construct ServiceTokenProvider
	tokenProvider, err := auth.NewServiceTokenProvider(auth.ServiceTokenProviderConfig{
		Secret:   testServiceSecret,
		Issuer:   testServiceIssuer,
		Audience: testServiceAudience,
		TTL:      5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("failed to construct ServiceTokenProvider: %v", err)
	}

	// 2. Construct userclient.HTTPClient
	userHTTPClient, err := userclient.NewClient(server.URL, server.Client(), tokenProvider)
	if err != nil {
		t.Fatalf("failed to construct userclient.HTTPClient: %v", err)
	}

	// 3. Construct user.Service
	userService, err := user.NewService(userHTTPClient)
	if err != nil {
		t.Fatalf("failed to construct user.Service: %v", err)
	}

	// 4. Resolve user via user.Service
	ctx := context.Background()
	resUser, err := userService.GetUserByID(ctx, "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d")
	if err != nil {
		t.Fatalf("failed to resolve user through composed stack: %v", err)
	}

	if resUser.ID != expectedUser.ID {
		t.Errorf("expected user ID %s, got: %s", expectedUser.ID, resUser.ID)
	}
	if resUser.Username != expectedUser.Username {
		t.Errorf("expected username %s, got: %s", expectedUser.Username, resUser.Username)
	}
	if !strings.HasPrefix(capturedAuth, "Bearer ") {
		t.Errorf("expected Authorization header with Bearer prefix, got: %s", capturedAuth)
	}
	if capturedAccept != "application/json" {
		t.Errorf("expected Accept application/json, got: %s", capturedAccept)
	}
}

func TestCompositionRoot_InvalidServiceTokenSecret(t *testing.T) {
	_, err := auth.NewServiceTokenProvider(auth.ServiceTokenProviderConfig{
		Secret:   "invalid-base64-secret!",
		Issuer:   testServiceIssuer,
		Audience: testServiceAudience,
	})
	if err == nil {
		t.Fatal("expected error for invalid service token secret, got nil")
	}
}

func TestCompositionRoot_InvalidBaseURL(t *testing.T) {
	tokenProvider, err := auth.NewServiceTokenProvider(auth.ServiceTokenProviderConfig{
		Secret:   testServiceSecret,
		Issuer:   testServiceIssuer,
		Audience: testServiceAudience,
	})
	if err != nil {
		t.Fatalf("unexpected error constructing token provider: %v", err)
	}

	_, err = userclient.NewClient("ftp://invalid-scheme.com", nil, tokenProvider)
	if err == nil {
		t.Fatal("expected error for invalid base URL scheme, got nil")
	}
}

func TestCompositionRoot_NilTokenProvider(t *testing.T) {
	_, err := userclient.NewClient("https://api.gmhelper.com", nil, nil)
	if !errors.Is(err, userclient.ErrMissingTokenProvider) {
		t.Fatalf("expected ErrMissingTokenProvider, got: %v", err)
	}
}

func TestCompositionRoot_NilUserClient(t *testing.T) {
	_, err := user.NewService(nil)
	if err == nil {
		t.Fatal("expected error constructing user.Service with nil client, got nil")
	}
}
