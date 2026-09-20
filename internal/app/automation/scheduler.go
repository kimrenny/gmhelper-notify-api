package automation

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gmhelper/notify-api/internal/infra/logger"
)

const (
	DefaultAutomationSchedulerInterval  = 1 * time.Hour
	DefaultAutomationSchedulerBatchSize = 250
)

// Scheduler periodically triggers automation inactivity evaluation against authoritative users.
type Scheduler struct {
	engine     *Engine
	userLister UserLister
	interval   time.Duration
	batchSize  int
	logger     logger.Logger
	nowFn      func() time.Time
	mu         sync.Mutex
	running    bool
}

// NewScheduler constructs a new automation Scheduler.
func NewScheduler(
	engine *Engine,
	userLister UserLister,
	interval time.Duration,
	batchSize int,
	log logger.Logger,
) *Scheduler {
	if interval <= 0 {
		interval = DefaultAutomationSchedulerInterval
	}
	if batchSize <= 0 {
		batchSize = DefaultAutomationSchedulerBatchSize
	}
	if log == nil {
		log = logger.NewNop()
	}
	return &Scheduler{
		engine:     engine,
		userLister: userLister,
		interval:   interval,
		batchSize:  batchSize,
		logger:     log,
		nowFn:      func() time.Time { return time.Now().UTC() },
	}
}

// SetNowFunc overrides the reference time generator (for deterministic testing).
func (s *Scheduler) SetNowFunc(fn func() time.Time) {
	if fn != nil {
		s.nowFn = fn
	}
}

// IsRunning returns whether the background loop is active.
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Start runs the periodic background evaluation loop until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		s.logger.Warn("automation background scheduler is already running")
		return
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		s.logger.Info("automation background scheduler stopped")
	}()

	s.logger.Info("automation background scheduler started",
		logger.Duration("interval", s.interval),
		logger.Int("batchSize", s.batchSize),
	)

	// Initial evaluation pass on startup
	if _, err := s.EvaluateDue(ctx); err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Warn("initial automation evaluation pass completed with error", logger.Error(err))
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.EvaluateDue(ctx); err != nil && !errors.Is(err, context.Canceled) {
				s.logger.Warn("periodic automation evaluation pass completed with error", logger.Error(err))
			}
		}
	}
}

// EvaluateDue runs a single evaluation pass across all users.
// Mutex protection ensures that one evaluation pass cannot overlap another.
func (s *Scheduler) EvaluateDue(ctx context.Context) (*InactivityEvaluationSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.engine == nil || s.userLister == nil {
		return nil, errors.New("automation engine or user lister is nil")
	}

	now := s.nowFn()
	s.logger.Debug("starting automation inactivity evaluation pass", logger.Time("now", now))

	summary, err := s.engine.EvaluateInactivity(ctx, s.userLister, now, s.batchSize)
	if err != nil {
		s.logger.Error("automation inactivity evaluation pass failed", logger.Error(err))
		return summary, err
	}

	s.logger.Info("automation inactivity evaluation pass finished",
		logger.Int("evaluatedUsers", summary.TotalUsersEvaluated),
		logger.Int("evaluatedRules", summary.TotalRulesEvaluated),
		logger.Int("executed", summary.ExecutedCount),
		logger.Int("skippedNotMatched", summary.SkippedNotMatchedCount),
		logger.Int("skippedCooldown", summary.SkippedCooldownCount),
		logger.Int("skippedDuplicate", summary.SkippedDuplicateCount),
		logger.Int("failed", summary.FailedCount),
	)

	return summary, nil
}
