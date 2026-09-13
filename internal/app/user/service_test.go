package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

type mockUserResolver struct {
	getUserByIDFunc func(ctx context.Context, id string) (*userclient.User, error)
	searchUsersFunc func(ctx context.Context, query string, limit int) ([]userclient.User, error)
}

func (m *mockUserResolver) GetUserByID(ctx context.Context, id string) (*userclient.User, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockUserResolver) SearchUsers(ctx context.Context, query string, limit int) ([]userclient.User, error) {
	if m.searchUsersFunc != nil {
		return m.searchUsersFunc(ctx, query, limit)
	}
	return nil, nil
}

func TestNewService(t *testing.T) {
	// 1. Nil resolver rejected
	svc, err := NewService(nil)
	if err == nil {
		t.Fatal("expected error when constructing Service with nil resolver, got nil")
	}
	if svc != nil {
		t.Errorf("expected nil service, got: %v", svc)
	}

	// 2. Valid resolver accepted
	mock := &mockUserResolver{}
	svc, err = NewService(mock)
	if err != nil {
		t.Fatalf("expected successful construction, got: %v", err)
	}
	if svc == nil || svc.resolver != mock {
		t.Errorf("expected service to hold injected resolver")
	}
}

func TestService_GetUserByID_Success(t *testing.T) {
	regDate := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	expectedUser := &userclient.User{
		ID:               "11111111-2222-3333-4444-555555555555",
		Username:         "alice",
		Email:            "alice@example.com",
		Role:             "Admin",
		Language:         "en",
		IsActive:         true,
		IsBlocked:        false,
		RegistrationDate: regDate,
	}

	var passedID string
	mock := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			passedID = id
			return expectedUser, nil
		},
	}

	svc, err := NewService(mock)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	user, err := svc.GetUserByID(context.Background(), " 11111111-2222-3333-4444-555555555555 ")
	if err != nil {
		t.Fatalf("expected successful resolution, got: %v", err)
	}

	if passedID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("expected trimmed ID passed to resolver, got: %s", passedID)
	}

	if user != expectedUser {
		t.Errorf("expected returned user %v, got %v", expectedUser, user)
	}
}

func TestService_GetUserByID_EmptyID(t *testing.T) {
	var resolverInvoked bool
	mock := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			resolverInvoked = true
			return nil, nil
		},
	}

	svc, _ := NewService(mock)

	// Empty string
	_, err := svc.GetUserByID(context.Background(), "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty ID, got: %v", err)
	}

	// Whitespace string
	_, err = svc.GetUserByID(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for whitespace ID, got: %v", err)
	}

	if resolverInvoked {
		t.Error("expected resolver NOT to be called when ID is invalid")
	}
}

func TestService_GetUserByID_ContextPropagation(t *testing.T) {
	type ctxKey struct{}
	var receivedCtx context.Context

	mock := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			receivedCtx = ctx
			return &userclient.User{ID: id}, nil
		},
	}

	svc, _ := NewService(mock)

	reqCtx := context.WithValue(context.Background(), ctxKey{}, "trace-123")
	_, err := svc.GetUserByID(reqCtx, "user-id-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedCtx == nil || receivedCtx.Value(ctxKey{}) != "trace-123" {
		t.Errorf("expected context with trace value to be forwarded to resolver")
	}
}

func TestService_GetUserByID_ContextCancellation(t *testing.T) {
	mock := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			return nil, ctx.Err()
		},
	}

	svc, _ := NewService(mock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before call

	_, err := svc.GetUserByID(ctx, "user-id-1")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

func TestService_GetUserByID_NotFound(t *testing.T) {
	mock := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			return nil, userclient.ErrNotFound
		},
	}

	svc, _ := NewService(mock)

	_, err := svc.GetUserByID(context.Background(), "non-existent-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
	if !errors.Is(err, userclient.ErrNotFound) {
		t.Errorf("expected userclient.ErrNotFound, got: %v", err)
	}
}

func TestService_GetUserByID_ResolverErrorPropagation(t *testing.T) {
	customErr := errors.New("upstream service unavailable")
	mock := &mockUserResolver{
		getUserByIDFunc: func(ctx context.Context, id string) (*userclient.User, error) {
			return nil, customErr
		},
	}

	svc, _ := NewService(mock)

	_, err := svc.GetUserByID(context.Background(), "user-id-1")
	if !errors.Is(err, customErr) {
		t.Errorf("expected error %v, got %v", customErr, err)
	}
}

func TestService_SearchUsers_Success(t *testing.T) {
	expected := []userclient.User{
		{ID: "u-1", Username: "alice", Email: "alice@example.com"},
		{ID: "u-2", Username: "alicia", Email: "alicia@example.com"},
	}

	var capturedQuery string
	var capturedLimit int

	mock := &mockUserResolver{
		searchUsersFunc: func(ctx context.Context, query string, limit int) ([]userclient.User, error) {
			capturedQuery = query
			capturedLimit = limit
			return expected, nil
		},
	}

	svc, _ := NewService(mock)

	users, err := svc.SearchUsers(context.Background(), "  alice  ", 25)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if capturedQuery != "alice" {
		t.Errorf("expected trimmed query 'alice', got '%s'", capturedQuery)
	}
	if capturedLimit != 25 {
		t.Errorf("expected limit 25, got %d", capturedLimit)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestService_SearchUsers_EmptyOrWhitespaceQuery(t *testing.T) {
	var invoked bool
	mock := &mockUserResolver{
		searchUsersFunc: func(ctx context.Context, query string, limit int) ([]userclient.User, error) {
			invoked = true
			return nil, nil
		},
	}

	svc, _ := NewService(mock)

	// Empty
	_, err := svc.SearchUsers(context.Background(), "", 20)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty query, got: %v", err)
	}

	// Whitespace
	_, err = svc.SearchUsers(context.Background(), "   \t\n   ", 20)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for whitespace query, got: %v", err)
	}

	if invoked {
		t.Error("expected resolver NOT to be invoked on invalid query")
	}
}

func TestService_SearchUsers_LimitClamping(t *testing.T) {
	var passedLimit int
	mock := &mockUserResolver{
		searchUsersFunc: func(ctx context.Context, query string, limit int) ([]userclient.User, error) {
			passedLimit = limit
			return []userclient.User{}, nil
		},
	}

	svc, _ := NewService(mock)

	// 1. Limit <= 0 -> default 20
	_, _ = svc.SearchUsers(context.Background(), "test", 0)
	if passedLimit != DefaultSearchLimit {
		t.Errorf("expected default limit %d for limit 0, got %d", DefaultSearchLimit, passedLimit)
	}

	_, _ = svc.SearchUsers(context.Background(), "test", -10)
	if passedLimit != DefaultSearchLimit {
		t.Errorf("expected default limit %d for negative limit, got %d", DefaultSearchLimit, passedLimit)
	}

	// 2. Limit > MaxSearchLimit (50) -> clamped to 50
	_, _ = svc.SearchUsers(context.Background(), "test", 100)
	if passedLimit != MaxSearchLimit {
		t.Errorf("expected clamped limit %d for limit 100, got %d", MaxSearchLimit, passedLimit)
	}

	// 3. Valid limit preserved
	_, _ = svc.SearchUsers(context.Background(), "test", 35)
	if passedLimit != 35 {
		t.Errorf("expected limit 35 to be preserved, got %d", passedLimit)
	}
}

func TestService_SearchUsers_ContextAndErrorPropagation(t *testing.T) {
	type traceKey struct{}
	var receivedCtx context.Context
	customErr := errors.New("upstream connection reset")

	mock := &mockUserResolver{
		searchUsersFunc: func(ctx context.Context, query string, limit int) ([]userclient.User, error) {
			receivedCtx = ctx
			return nil, customErr
		},
	}

	svc, _ := NewService(mock)

	reqCtx := context.WithValue(context.Background(), traceKey{}, "trace-val")
	_, err := svc.SearchUsers(reqCtx, "alice", 20)

	if !errors.Is(err, customErr) {
		t.Errorf("expected error %v, got: %v", customErr, err)
	}
	if receivedCtx == nil || receivedCtx.Value(traceKey{}) != "trace-val" {
		t.Errorf("expected context with trace value to be passed to resolver")
	}
}
