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
}

func (m *mockUserResolver) GetUserByID(ctx context.Context, id string) (*userclient.User, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, id)
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
