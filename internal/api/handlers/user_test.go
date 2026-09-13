package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/user"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

type handlerMockUserResolver struct {
	searchUsersFunc func(ctx context.Context, query string, limit int) ([]userclient.User, error)
	getUserByIDFunc func(ctx context.Context, id string) (*userclient.User, error)
}

func (m *handlerMockUserResolver) SearchUsers(ctx context.Context, query string, limit int) ([]userclient.User, error) {
	if m.searchUsersFunc != nil {
		return m.searchUsersFunc(ctx, query, limit)
	}
	return nil, nil
}

func (m *handlerMockUserResolver) GetUserByID(ctx context.Context, id string) (*userclient.User, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, id)
	}
	return nil, nil
}

func TestUserHandler_Search_Success_MultipleUsers(t *testing.T) {
	log, _ := logger.NewLogger("info")
	regDate := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)

	expectedUsers := []userclient.User{
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
			Username:         "alice_cooper",
			Email:            "cooper@example.com",
			Role:             "User",
			Language:         "RU",
			IsActive:         false,
			IsBlocked:        true,
			RegistrationDate: regDate,
		},
	}

	var capturedQuery string
	var capturedLimit int

	resolver := &handlerMockUserResolver{
		searchUsersFunc: func(ctx context.Context, query string, limit int) ([]userclient.User, error) {
			capturedQuery = query
			capturedLimit = limit
			return expectedUsers, nil
		},
	}

	userService, _ := user.NewService(resolver)
	handler := NewUserHandler(userService, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/search?q=alice&limit=15", nil)
	rec := httptest.NewRecorder()

	handler.Search(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	if capturedQuery != "alice" {
		t.Errorf("expected query 'alice', got '%s'", capturedQuery)
	}
	if capturedLimit != 15 {
		t.Errorf("expected limit 15, got %d", capturedLimit)
	}

	var results []UserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 users, got %d", len(results))
	}

	if results[0].ID != "11111111-1111-1111-1111-111111111111" || results[0].Username != "alice" || results[0].Email != "alice@example.com" {
		t.Errorf("unexpected user 0: %+v", results[0])
	}
	if results[1].ID != "22222222-2222-2222-2222-222222222222" || results[1].Username != "alice_cooper" || results[1].Role != "User" {
		t.Errorf("unexpected user 1: %+v", results[1])
	}
}

func TestUserHandler_Search_Success_EmptyResults(t *testing.T) {
	log, _ := logger.NewLogger("info")

	resolver := &handlerMockUserResolver{
		searchUsersFunc: func(ctx context.Context, query string, limit int) ([]userclient.User, error) {
			return []userclient.User{}, nil
		},
	}

	userService, _ := user.NewService(resolver)
	handler := NewUserHandler(userService, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/search?q=nonexistent", nil)
	rec := httptest.NewRecorder()

	handler.Search(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var results []UserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected empty array, got %d items", len(results))
	}
}

func TestUserHandler_Search_MissingQuery(t *testing.T) {
	log, _ := logger.NewLogger("info")

	resolver := &handlerMockUserResolver{}
	userService, _ := user.NewService(resolver)
	handler := NewUserHandler(userService, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/search", nil)
	rec := httptest.NewRecorder()

	handler.Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing query, got %d", rec.Code)
	}

	var errResp response.ErrorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "BAD_REQUEST" {
		t.Errorf("expected error code BAD_REQUEST, got %s", errResp.Error.Code)
	}
}

func TestUserHandler_Search_WhitespaceQuery(t *testing.T) {
	log, _ := logger.NewLogger("info")

	resolver := &handlerMockUserResolver{}
	userService, _ := user.NewService(resolver)
	handler := NewUserHandler(userService, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/search?q=%20%20%20", nil)
	rec := httptest.NewRecorder()

	handler.Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for whitespace query, got %d", rec.Code)
	}
}

func TestUserHandler_Search_InvalidLimit(t *testing.T) {
	log, _ := logger.NewLogger("info")

	resolver := &handlerMockUserResolver{}
	userService, _ := user.NewService(resolver)
	handler := NewUserHandler(userService, log)

	// Non-numeric limit
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/search?q=test&limit=abc", nil)
	rec := httptest.NewRecorder()
	handler.Search(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-numeric limit, got %d", rec.Code)
	}

	// Negative limit
	reqNeg := httptest.NewRequest(http.MethodGet, "/api/v1/users/search?q=test&limit=-5", nil)
	recNeg := httptest.NewRecorder()
	handler.Search(recNeg, reqNeg)
	if recNeg.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative limit, got %d", recNeg.Code)
	}
}

func TestUserHandler_Search_UpstreamError(t *testing.T) {
	log, _ := logger.NewLogger("info")

	resolver := &handlerMockUserResolver{
		searchUsersFunc: func(ctx context.Context, query string, limit int) ([]userclient.User, error) {
			return nil, errors.New("upstream connection reset")
		},
	}

	userService, _ := user.NewService(resolver)
	handler := NewUserHandler(userService, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/search?q=alice", nil)
	rec := httptest.NewRecorder()

	handler.Search(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected status 502 Bad Gateway on upstream error, got %d", rec.Code)
	}

	var errResp response.ErrorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "UPSTREAM_ERROR" {
		t.Errorf("expected error code UPSTREAM_ERROR, got %s", errResp.Error.Code)
	}
}

func TestUserHandler_Search_ServiceUnavailable_WhenNilService(t *testing.T) {
	log, _ := logger.NewLogger("info")

	handler := NewUserHandler(nil, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/search?q=alice", nil)
	rec := httptest.NewRecorder()

	handler.Search(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 Service Unavailable, got %d", rec.Code)
	}
}
