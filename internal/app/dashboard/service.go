package dashboard

import (
	"context"
	"errors"

	"github.com/gmhelper/notify-api/internal/domain"
)

const (
	DefaultRecentLimit = 5
	MaxRecentLimit     = 50
)

var (
	ErrRepositoryNil = errors.New("dashboard repository cannot be nil")
)

// Service provides dashboard business logic and statistics retrieval.
type Service struct {
	repo domain.DashboardRepository
}

// NewService constructs a new Dashboard application service.
func NewService(repo domain.DashboardRepository) *Service {
	return &Service{repo: repo}
}

// GetStats returns aggregated dashboard statistics with a validated recent-campaign limit.
func (s *Service) GetStats(ctx context.Context, recentLimit int) (*domain.DashboardStats, error) {
	if s.repo == nil {
		return nil, ErrRepositoryNil
	}

	if recentLimit <= 0 {
		recentLimit = DefaultRecentLimit
	} else if recentLimit > MaxRecentLimit {
		recentLimit = MaxRecentLimit
	}

	return s.repo.GetDashboardStats(ctx, recentLimit)
}
