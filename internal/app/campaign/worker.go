package campaign

import (
	"context"
	"sync"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

const (
	defaultDeliveryWorkerInterval     = 5 * time.Second
	defaultDeliveryWorkerBatchSize    = 20
	defaultDeliveryWorkerStaleTimeout = 5 * time.Minute
)

// Worker periodically claims pending campaign recipients for delivery and finalizes completed campaigns.
type Worker struct {
	recipientRepo   domain.CampaignRecipientRepository
	campaignRepo    domain.NotificationCampaignRepository
	deliveryService *DeliveryService
	interval        time.Duration
	staleTimeout    time.Duration
	batchSize       int
	logger          logger.Logger
	mu              sync.Mutex
	running         bool
}

// NewWorker constructs a new campaign delivery Worker.
func NewWorker(
	recipientRepo domain.CampaignRecipientRepository,
	campaignRepo domain.NotificationCampaignRepository,
	deliveryService *DeliveryService,
	interval time.Duration,
	staleTimeout time.Duration,
	batchSize int,
	logger logger.Logger,
) *Worker {
	if interval <= 0 {
		interval = defaultDeliveryWorkerInterval
	}
	if staleTimeout <= 0 {
		staleTimeout = defaultDeliveryWorkerStaleTimeout
	}
	if batchSize <= 0 {
		batchSize = defaultDeliveryWorkerBatchSize
	}
	return &Worker{
		recipientRepo:   recipientRepo,
		campaignRepo:    campaignRepo,
		deliveryService: deliveryService,
		interval:        interval,
		staleTimeout:    staleTimeout,
		batchSize:       batchSize,
		logger:          logger,
	}
}

// Start begins the background polling loop and runs until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		if w.logger != nil {
			w.logger.Warn("campaign delivery worker is already running")
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
			w.logger.Info("campaign delivery background worker stopped")
		}
	}()

	if w.logger != nil {
		w.logger.Info("campaign delivery background worker started",
			logger.Duration("interval", w.interval),
			logger.Duration("staleTimeout", w.staleTimeout),
			logger.Int("batchSize", w.batchSize),
		)
	}

	// 1. Initial pass
	w.ProcessPending(ctx)

	// 2. Polling loop
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

// ProcessPending performs recovery of stale sending recipients, claims a batch of pending recipients, executes delivery, and finalizes completed campaigns.
func (w *Worker) ProcessPending(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	// 1. Recover stale sending recipients
	if w.staleTimeout > 0 {
		recovered, err := w.recipientRepo.RecoverStaleSending(ctx, w.staleTimeout)
		if err != nil {
			if ctx.Err() == nil && w.logger != nil {
				w.logger.Warn("failed to recover stale sending campaign recipients", logger.Error(err))
			}
		} else if recovered > 0 && w.logger != nil {
			w.logger.Info("recovered stale sending campaign recipients", logger.Int64("count", recovered))
		}
	}

	// 2. Claim batch of pending recipients
	claimed, err := w.recipientRepo.ClaimPending(ctx, w.batchSize)
	if err != nil {
		if ctx.Err() == nil && w.logger != nil {
			w.logger.Error("failed to claim pending campaign recipients", logger.Error(err))
		}
		return
	}

	campaignsToCheck := make(map[string]struct{})

	for _, recipient := range claimed {
		if ctx.Err() != nil {
			return
		}

		campaignsToCheck[recipient.CampaignID] = struct{}{}

		if err := w.deliveryService.DeliverClaimed(ctx, recipient); err != nil {
			if w.logger != nil {
				w.logger.Warn("failed to deliver campaign recipient",
					logger.String("recipientId", recipient.ID),
					logger.String("campaignId", recipient.CampaignID),
					logger.Error(err),
				)
			}
			// Error isolation: continue to next recipient
			continue
		}
	}

	// Also include any currently running campaigns to check for 0-recipient or finished campaigns
	runningCampaigns, err := w.campaignRepo.ListByStatus(ctx, domain.CampaignStatusRunning)
	if err == nil {
		for _, c := range runningCampaigns {
			campaignsToCheck[c.ID] = struct{}{}
		}
	}

	// 3. Finalize any completed campaigns
	for campaignID := range campaignsToCheck {
		if ctx.Err() != nil {
			return
		}
		finalized, finalStatus, err := w.deliveryService.FinalizeCampaignIfDone(ctx, campaignID)
		if err != nil {
			if w.logger != nil {
				w.logger.Error("failed to check campaign finalization",
					logger.String("campaignId", campaignID),
					logger.Error(err),
				)
			}
		} else if finalized && w.logger != nil {
			w.logger.Info("campaign execution finalized",
				logger.String("campaignId", campaignID),
				logger.String("finalStatus", string(finalStatus)),
			)
		}
	}
}
