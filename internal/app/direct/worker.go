package direct

import (
	"context"
	"sync"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

const defaultBatchSize = 10

// Worker periodically queries for pending direct notifications and delivers them via DeliveryService.
type Worker struct {
	directRepo      domain.DirectNotificationRepository
	deliveryService *DeliveryService
	interval        time.Duration
	batchSize       int
	logger          logger.Logger
	mu              sync.Mutex
	running         bool
}

// NewWorker initializes a new background delivery worker.
func NewWorker(
	directRepo domain.DirectNotificationRepository,
	deliveryService *DeliveryService,
	interval time.Duration,
	logger logger.Logger,
) *Worker {
	return &Worker{
		directRepo:      directRepo,
		deliveryService: deliveryService,
		interval:        interval,
		batchSize:       defaultBatchSize,
		logger:          logger,
	}
}

// Start begins the background polling loop. It performs an immediate initial pass and runs until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		if w.logger != nil {
			w.logger.Warn("direct notification worker is already running")
		}
		return
	}
	w.running = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
		if w.logger != nil {
			w.logger.Info("direct notification background worker stopped")
		}
	}()

	if w.logger != nil {
		w.logger.Info("direct notification background worker started",
			logger.Duration("interval", w.interval),
		)
	}

	// 1. Initial processing pass on startup
	w.ProcessPending(ctx)

	// 2. Periodic polling ticks
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ProcessPending(ctx)
		}
	}
}

// ProcessPending atomically claims a batch of pending notifications and processes each sequentially.
func (w *Worker) ProcessPending(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	claimed, err := w.directRepo.ClaimPending(ctx, w.batchSize)
	if err != nil {
		if ctx.Err() == nil && w.logger != nil {
			w.logger.Error("failed to claim pending direct notifications in worker", logger.Error(err))
		}
		return
	}

	if len(claimed) == 0 {
		return
	}

	if w.logger != nil {
		w.logger.Info("claimed pending direct notifications for delivery", logger.Int("count", len(claimed)))
	}

	for _, item := range claimed {
		if ctx.Err() != nil {
			return
		}

		if err := w.deliveryService.DeliverClaimed(ctx, item); err != nil {
			if w.logger != nil {
				w.logger.Warn("failed to deliver direct notification in worker",
					logger.String("id", item.ID),
					logger.Error(err),
				)
			}
			// Continue to next notification even if one delivery fails
			continue
		}

		if w.logger != nil {
			w.logger.Info("successfully delivered direct notification in worker",
				logger.String("id", item.ID),
			)
		}
	}
}
