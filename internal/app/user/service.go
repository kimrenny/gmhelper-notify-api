package user

import (
	"context"
	"errors"
	"strings"

	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

const (
	DefaultSearchLimit = 20
	MaxSearchLimit     = 50
)

var (
	ErrInvalidInput = errors.New("invalid user id")
	ErrNotFound     = userclient.ErrNotFound
)

// UserResolver defines the abstraction for resolving user information from gmhelper-api.
type UserResolver interface {
	GetUserByID(ctx context.Context, id string) (*userclient.User, error)
	SearchUsers(ctx context.Context, query string, limit int) ([]userclient.User, error)
}

// Service provides application-level user resolution logic.
type Service struct {
	resolver UserResolver
}

// NewService constructs a new application user Service with the given UserResolver.
func NewService(resolver UserResolver) (*Service, error) {
	if resolver == nil {
		return nil, errors.New("user resolver cannot be nil")
	}
	return &Service{
		resolver: resolver,
	}, nil
}

// GetUserByID resolves a user profile by ID using the underlying resolver.
func (s *Service) GetUserByID(ctx context.Context, id string) (*userclient.User, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return nil, ErrInvalidInput
	}
	return s.resolver.GetUserByID(ctx, trimmedID)
}

// SearchUsers searches users by username or email using the underlying resolver.
func (s *Service) SearchUsers(ctx context.Context, query string, limit int) ([]userclient.User, error) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil, ErrInvalidInput
	}

	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = DefaultSearchLimit
	} else if effectiveLimit > MaxSearchLimit {
		effectiveLimit = MaxSearchLimit
	}

	return s.resolver.SearchUsers(ctx, trimmedQuery, effectiveLimit)
}
