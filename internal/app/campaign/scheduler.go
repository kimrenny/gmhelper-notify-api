package campaign

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

const (
	defaultSchedulerInterval  = 5 * time.Second
	defaultSchedulerBatchSize = 10
)

// Scheduler periodically discovers due scheduled campaigns and atomically claims them into running state.
type Scheduler struct {
	repo      domain.NotificationCampaignRepository
	populator AudiencePopulator
	interval  time.Duration
	batchSize int
	logger    logger.Logger
	nowFn     func() time.Time
	mu        sync.Mutex
	running   bool
}

// NewScheduler initializes a new Campaign Scheduler.
func NewScheduler(
	repo domain.NotificationCampaignRepository,
	populator AudiencePopulator,
	interval time.Duration,
	batchSize int,
	logger logger.Logger,
) *Scheduler {
	if interval <= 0 {
		interval = defaultSchedulerInterval
	}
	if batchSize <= 0 {
		batchSize = defaultSchedulerBatchSize
	}
	return &Scheduler{
		repo:      repo,
		populator: populator,
		interval:  interval,
		batchSize: batchSize,
		logger:    logger,
		nowFn:     func() time.Time { return time.Now().UTC() },
	}
}

// Start begins the background polling loop until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		if s.logger != nil {
			s.logger.Warn("campaign scheduler is already running")
		}
		return
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		if s.logger != nil {
			s.logger.Info("campaign background scheduler stopped")
		}
	}()

	if s.logger != nil {
		s.logger.Info("campaign background scheduler started",
			logger.Duration("interval", s.interval),
			logger.Int("batchSize", s.batchSize),
		)
	}

	// 1. Initial processing pass on startup
	s.ProcessDue(ctx)

	// 2. Periodic polling ticks
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ProcessDue(ctx)
		}
	}
}

// ProcessDue checks for due scheduled campaigns and claims them for execution.
func (s *Scheduler) ProcessDue(ctx context.Context) (int, error) {
	now := s.nowFn()

	dueCampaigns, err := s.repo.ListDue(ctx, now, s.batchSize)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to list due campaigns", logger.Error(err))
		}
		return 0, err
	}

	if len(dueCampaigns) == 0 {
		return 0, nil
	}

	if s.logger != nil {
		s.logger.Info("discovered due campaigns", logger.Int("count", len(dueCampaigns)))
	}

	claimedCount := 0
	for _, c := range dueCampaigns {
		if ctx.Err() != nil {
			return claimedCount, ctx.Err()
		}

		claimed, err := s.repo.Claim(ctx, c.ID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict) {
				if s.logger != nil {
					s.logger.Debug("campaign already claimed or transitioned by another instance",
						logger.String("id", c.ID),
					)
				}
				continue
			}
			if s.logger != nil {
				s.logger.Error("failed to claim campaign for execution",
					logger.String("id", c.ID),
					logger.Error(err),
				)
			}
			continue
		}

		claimedCount++
		if s.logger != nil {
			s.logger.Info("successfully claimed campaign for execution",
				logger.String("id", claimed.ID),
				logger.String("name", claimed.Name),
				logger.String("status", string(claimed.Status)),
			)
		}

		if s.populator != nil {
			if _, popErr := s.populator.PopulateAudience(ctx, claimed.ID); popErr != nil {
				if s.logger != nil {
					s.logger.Error("failed to populate audience for claimed campaign",
						logger.String("id", claimed.ID),
						logger.Error(popErr),
					)
				}
			}
		} else {
			if s.logger != nil {
				s.logger.Warn("audience populator is not configured (GMHELPER_API_BASE_URL may be missing); audience population skipped for claimed campaign",
					logger.String("id", claimed.ID),
				)
			}
		}
	}

	return claimedCount, nil
}
