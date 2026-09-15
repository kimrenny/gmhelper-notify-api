package campaign

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

func TestWorker_ProcessPending_SuccessAndFinalization(t *testing.T) {
	ctx := context.Background()
	cmpID := "cmp-w-1"
	tplID := "tpl-w-1"

	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			cmpID: {ID: cmpID, TemplateID: tplID, Status: domain.CampaignStatusRunning},
		},
	}
	tRepo := newMockTemplateRepo()
	tRepo.templates[tplID] = &domain.EmailTemplate{
		ID:            tplID,
		TemplateKey:   "tpl-w-1",
		Subject:       "Hi {{username}}",
		HTMLBody:      "<p>Hi</p>",
		PlainTextBody: "Hi",
		Status:        domain.TemplateStatusActive,
	}
	rRepo := newMockRecipientRepo()
	rRepo.recipients["r1"] = &domain.CampaignRecipient{
		ID:             "r1",
		CampaignID:     cmpID,
		RecipientEmail: "user1@example.com",
		RecipientName:  "User1",
		DeliveryStatus: domain.DeliveryStatusPending,
	}
	rRepo.recipients["r2"] = &domain.CampaignRecipient{
		ID:             "r2",
		CampaignID:     cmpID,
		RecipientEmail: "user2@example.com",
		RecipientName:  "User2",
		DeliveryStatus: domain.DeliveryStatusPending,
	}
	aRepo := newMockAttemptRepo()
	sender := &mockSender{}
	log, _ := logger.NewLogger("error")

	deliverySvc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, sender, nil)
	worker := NewWorker(rRepo, cRepo, deliverySvc, 100*time.Millisecond, 5*time.Minute, 10, log)

	worker.ProcessPending(ctx)

	// Verify both recipients delivered
	if rRepo.recipients["r1"].DeliveryStatus != domain.DeliveryStatusSent {
		t.Fatalf("expected r1 sent, got %s", rRepo.recipients["r1"].DeliveryStatus)
	}
	if rRepo.recipients["r2"].DeliveryStatus != domain.DeliveryStatusSent {
		t.Fatalf("expected r2 sent, got %s", rRepo.recipients["r2"].DeliveryStatus)
	}
	if len(sender.sent) != 2 {
		t.Fatalf("expected 2 emails sent, got %d", len(sender.sent))
	}

	// Verify campaign finalized to completed
	if cRepo.campaigns[cmpID].Status != domain.CampaignStatusCompleted {
		t.Fatalf("expected campaign status completed, got %s", cRepo.campaigns[cmpID].Status)
	}
}

func TestWorker_ProcessPending_ErrorIsolation(t *testing.T) {
	ctx := context.Background()
	cmpID := "cmp-w-err"
	tplID := "tpl-w-err"

	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			cmpID: {ID: cmpID, TemplateID: tplID, Status: domain.CampaignStatusRunning},
		},
	}
	tRepo := newMockTemplateRepo()
	tRepo.templates[tplID] = &domain.EmailTemplate{
		ID:            tplID,
		TemplateKey:   "tpl-w-err",
		Subject:       "Hi",
		HTMLBody:      "<p>Hi</p>",
		PlainTextBody: "Hi",
		Status:        domain.TemplateStatusActive,
	}
	rRepo := newMockRecipientRepo()
	rRepo.recipients["r1"] = &domain.CampaignRecipient{
		ID:             "r1",
		CampaignID:     cmpID,
		RecipientEmail: "fail@example.com",
		RecipientName:  "FailUser",
		DeliveryStatus: domain.DeliveryStatusPending,
	}
	rRepo.recipients["r2"] = &domain.CampaignRecipient{
		ID:             "r2",
		CampaignID:     cmpID,
		RecipientEmail: "ok@example.com",
		RecipientName:  "OkUser",
		DeliveryStatus: domain.DeliveryStatusPending,
	}
	aRepo := newMockAttemptRepo()

	// Fail specifically for r1 email
	deliverySvc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, &conditionalSender{
		sendFunc: func(ctx context.Context, msg *email.Message) error {
			if msg.To == "fail@example.com" {
				return errors.New("smtp fail")
			}
			return nil
		},
	}, nil)
	log, _ := logger.NewLogger("error")
	worker := NewWorker(rRepo, cRepo, deliverySvc, 100*time.Millisecond, 5*time.Minute, 10, log)

	worker.ProcessPending(ctx)

	// Verify r1 failed, r2 sent (error isolation!)
	if rRepo.recipients["r1"].DeliveryStatus != domain.DeliveryStatusFailed {
		t.Fatalf("expected r1 failed, got %s", rRepo.recipients["r1"].DeliveryStatus)
	}
	if rRepo.recipients["r2"].DeliveryStatus != domain.DeliveryStatusSent {
		t.Fatalf("expected r2 sent, got %s", rRepo.recipients["r2"].DeliveryStatus)
	}

	// Verify campaign finalized to partially_failed
	if cRepo.campaigns[cmpID].Status != domain.CampaignStatusPartiallyFailed {
		t.Fatalf("expected campaign status partially_failed, got %s", cRepo.campaigns[cmpID].Status)
	}
}

func TestWorker_ProcessPending_StaleRecovery(t *testing.T) {
	ctx := context.Background()
	cmpID := "cmp-w-stale"
	tplID := "tpl-w-stale"

	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			cmpID: {ID: cmpID, TemplateID: tplID, Status: domain.CampaignStatusRunning},
		},
	}
	tRepo := newMockTemplateRepo()
	tRepo.templates[tplID] = &domain.EmailTemplate{
		ID:            tplID,
		TemplateKey:   "tpl-w-stale",
		Subject:       "Hi",
		HTMLBody:      "<p>Hi</p>",
		PlainTextBody: "Hi",
		Status:        domain.TemplateStatusActive,
	}
	rRepo := newMockRecipientRepo()
	staleTime := time.Now().UTC().Add(-10 * time.Minute)
	rRepo.recipients["r-stale"] = &domain.CampaignRecipient{
		ID:             "r-stale",
		CampaignID:     cmpID,
		RecipientEmail: "stale@example.com",
		DeliveryStatus: domain.DeliveryStatusSending, // Stuck in sending
		LastAttemptAt:  &staleTime,
	}
	aRepo := newMockAttemptRepo()
	sender := &mockSender{}
	log, _ := logger.NewLogger("error")

	deliverySvc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, sender, nil)
	worker := NewWorker(rRepo, cRepo, deliverySvc, 100*time.Millisecond, 5*time.Minute, 10, log)

	worker.ProcessPending(ctx)

	// Stale item recovered to pending, then claimed and delivered to sent
	if rRepo.recipients["r-stale"].DeliveryStatus != domain.DeliveryStatusSent {
		t.Fatalf("expected recovered and sent, got %s", rRepo.recipients["r-stale"].DeliveryStatus)
	}
}

func TestWorker_StartAndCancel(t *testing.T) {
	cRepo := &mockRepo{campaigns: make(map[string]*domain.NotificationCampaign)}
	rRepo := newMockRecipientRepo()
	tRepo := newMockTemplateRepo()
	aRepo := newMockAttemptRepo()
	sender := &mockSender{}
	log, _ := logger.NewLogger("error")

	deliverySvc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, sender, nil)
	worker := NewWorker(rRepo, cRepo, deliverySvc, 10*time.Millisecond, 5*time.Minute, 10, log)

	ctx, cancel := context.WithCancel(context.Background())

	stopped := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(stopped)
	}()

	// Allow loop to tick
	time.Sleep(30 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case <-stopped:
		// Success: worker stopped cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("worker did not shut down in a timely manner")
	}
}

type conditionalSender struct {
	sendFunc func(ctx context.Context, msg *email.Message) error
}

func (c *conditionalSender) Send(ctx context.Context, msg *email.Message) error {
	if c.sendFunc != nil {
		return c.sendFunc(ctx, msg)
	}
	return nil
}
