package direct

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/domain"
)

type mockSender struct {
	sentMessages []*email.Message
	sendErr      error
}

func (m *mockSender) Send(ctx context.Context, message *email.Message) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMessages = append(m.sentMessages, message)
	return nil
}

type mockAttemptRepo struct {
	attempts map[string]*domain.DeliveryAttempt
}

func (m *mockAttemptRepo) GetByID(ctx context.Context, id string) (*domain.DeliveryAttempt, error) {
	att, ok := m.attempts[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return att, nil
}

func (m *mockAttemptRepo) Create(ctx context.Context, attempt *domain.DeliveryAttempt) error {
	m.attempts[attempt.ID] = attempt
	return nil
}

func (m *mockAttemptRepo) Update(ctx context.Context, attempt *domain.DeliveryAttempt) error {
	if _, ok := m.attempts[attempt.ID]; !ok {
		return domain.ErrNotFound
	}
	m.attempts[attempt.ID] = attempt
	return nil
}

func (m *mockAttemptRepo) ListByTarget(ctx context.Context, targetType domain.DeliveryTargetType, targetID string) ([]*domain.DeliveryAttempt, error) {
	var list []*domain.DeliveryAttempt
	for _, att := range m.attempts {
		if att.TargetType == targetType && att.TargetID == targetID {
			list = append(list, att)
		}
	}
	return list, nil
}

func setupTestDeliveryService() (*DeliveryService, *mockDirectRepo, *mockAttemptRepo, *mockTemplateRepo, *mockSender) {
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	attemptRepo := &mockAttemptRepo{
		attempts: make(map[string]*domain.DeliveryAttempt),
	}
	templateRepo := &mockTemplateRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
	sender := &mockSender{}

	svc := NewDeliveryService(directRepo, attemptRepo, templateRepo, sender, nil)
	return svc, directRepo, attemptRepo, templateRepo, sender
}

func TestDeliveryService_Deliver_Success(t *testing.T) {
	svc, directRepo, attemptRepo, templateRepo, sender := setupTestDeliveryService()

	// 1. Setup active template
	tpl := &domain.EmailTemplate{
		ID:            "tpl-100",
		TemplateKey:   "order_confirmation",
		Name:          "Order Confirmation",
		Subject:       "Order {{orderId}} Confirmed",
		HTMLBody:      "<p>Thank you, {{customerName}}!</p>",
		PlainTextBody: "Thank you, {{customerName}}!",
		Status:        domain.TemplateStatusActive,
		Version:       1,
	}
	templateRepo.templates[tpl.ID] = tpl

	// 2. Setup pending notification
	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{
		"orderId":      "ORD-1234",
		"customerName": "Renny",
	})
	notif := &domain.DirectNotification{
		ID:             "notif-100",
		TemplateID:     tpl.ID,
		RecipientEmail: "renny@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
		Payload:        payload,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	directRepo.notifications[notif.ID] = notif

	// 3. Setup initial pending delivery attempt
	attempt := &domain.DeliveryAttempt{
		ID:            "attempt-100",
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      notif.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
		AttemptedAt:   now,
		CreatedAt:     now,
	}
	attemptRepo.attempts[attempt.ID] = attempt

	// 4. Deliver
	if err := svc.Deliver(context.Background(), notif.ID); err != nil {
		t.Fatalf("expected Deliver success, got: %v", err)
	}

	// Verify SMTP message sent
	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected 1 SMTP message sent, got %d", len(sender.sentMessages))
	}
	msg := sender.sentMessages[0]
	if msg.To != "renny@example.com" {
		t.Errorf("expected To renny@example.com, got %s", msg.To)
	}
	if msg.Subject != "Order ORD-1234 Confirmed" {
		t.Errorf("expected rendered subject, got: %s", msg.Subject)
	}

	// Verify Notification status updated to Sent
	if notif.DeliveryStatus != domain.DeliveryStatusSent {
		t.Errorf("expected notification status sent, got %s", notif.DeliveryStatus)
	}
	if notif.SentAt == nil {
		t.Error("expected sent_at to be set on notification")
	}
	if notif.AttemptsCount != 1 {
		t.Errorf("expected attempts_count 1, got %d", notif.AttemptsCount)
	}

	// Verify Attempt updated to Sent
	if attempt.Status != domain.DeliveryStatusSent {
		t.Errorf("expected attempt status sent, got %s", attempt.Status)
	}
	if attempt.ErrorMessage != "" {
		t.Errorf("expected empty error message on attempt, got: %s", attempt.ErrorMessage)
	}
}

func TestDeliveryService_Deliver_SMTPFailure(t *testing.T) {
	svc, directRepo, attemptRepo, templateRepo, sender := setupTestDeliveryService()

	tpl := &domain.EmailTemplate{
		ID:       "tpl-101",
		Subject:  "Subject",
		HTMLBody: "<p>Body</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	templateRepo.templates[tpl.ID] = tpl

	notif := &domain.DirectNotification{
		ID:             "notif-101",
		TemplateID:     tpl.ID,
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
	}
	directRepo.notifications[notif.ID] = notif

	attempt := &domain.DeliveryAttempt{
		ID:            "attempt-101",
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      notif.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
	}
	attemptRepo.attempts[attempt.ID] = attempt

	// Simulate SMTP send error
	smtpErr := errors.New("550 5.1.1 User unknown")
	sender.sendErr = smtpErr

	err := svc.Deliver(context.Background(), notif.ID)
	if err == nil {
		t.Fatal("expected Deliver error on SMTP failure, got nil")
	}
	if !errors.Is(err, smtpErr) {
		t.Errorf("expected returned error to wrap/match original smtpErr, got %v", err)
	}

	// 1. Below max attempts (attempt 1 of 5) -> notification returned to pending for retry
	if notif.DeliveryStatus != domain.DeliveryStatusPending {
		t.Errorf("expected notification status pending for retry below max attempts, got %s", notif.DeliveryStatus)
	}
	if notif.AttemptsCount != 1 {
		t.Errorf("expected attempts_count 1, got %d", notif.AttemptsCount)
	}
	if notif.ErrorMessage != "550 5.1.1 User unknown" {
		t.Errorf("expected notification error_message recorded, got: %s", notif.ErrorMessage)
	}
	if notif.SentAt != nil {
		t.Error("expected sent_at to remain nil on failure")
	}

	// Verify Attempt marked as failed with error message
	if attempt.Status != domain.DeliveryStatusFailed {
		t.Errorf("expected attempt status failed, got %s", attempt.Status)
	}
	if attempt.ErrorMessage != "550 5.1.1 User unknown" {
		t.Errorf("expected attempt error_message recorded, got: %s", attempt.ErrorMessage)
	}

	// 2. At max attempts (e.g. maxAttempts = 1 or notification already at 4 attempts) -> permanently failed
	svcMax1 := NewDeliveryServiceWithMaxAttempts(directRepo, attemptRepo, templateRepo, sender, 1, nil)
	notif2 := &domain.DirectNotification{
		ID:             "notif-max-reached",
		TemplateID:     tpl.ID,
		RecipientEmail: "user2@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
	}
	directRepo.notifications[notif2.ID] = notif2
	attempt2 := &domain.DeliveryAttempt{
		ID:            "attempt-max-reached",
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      notif2.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
	}
	attemptRepo.attempts[attempt2.ID] = attempt2

	err = svcMax1.Deliver(context.Background(), notif2.ID)
	if err == nil {
		t.Fatal("expected error on SMTP failure, got nil")
	}
	if notif2.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Errorf("expected notification to be permanently failed when reaching max attempts, got %s", notif2.DeliveryStatus)
	}
}

func TestDeliveryService_Deliver_InvalidStates(t *testing.T) {
	svc, directRepo, _, templateRepo, _ := setupTestDeliveryService()

	tpl := &domain.EmailTemplate{
		ID:       "tpl-1",
		Subject:  "Subject",
		HTMLBody: "<p>Body</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	templateRepo.templates[tpl.ID] = tpl

	// 1. Already Sent
	notifSent := &domain.DirectNotification{
		ID:             "notif-sent",
		TemplateID:     tpl.ID,
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusSent,
	}
	directRepo.notifications[notifSent.ID] = notifSent

	err := svc.Deliver(context.Background(), notifSent.ID)
	if !errors.Is(err, ErrAlreadySent) {
		t.Errorf("expected ErrAlreadySent, got %v", err)
	}

	// 2. Cancelled
	notifCancelled := &domain.DirectNotification{
		ID:             "notif-cancelled",
		TemplateID:     tpl.ID,
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusCancelled,
	}
	directRepo.notifications[notifCancelled.ID] = notifCancelled

	err = svc.Deliver(context.Background(), notifCancelled.ID)
	if !errors.Is(err, ErrNotificationCancelled) {
		t.Errorf("expected ErrNotificationCancelled, got %v", err)
	}

	// 3. Already Failed
	notifFailed := &domain.DirectNotification{
		ID:             "notif-failed",
		TemplateID:     tpl.ID,
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusFailed,
		AttemptsCount:  5,
	}
	directRepo.notifications[notifFailed.ID] = notifFailed

	err = svc.Deliver(context.Background(), notifFailed.ID)
	if !errors.Is(err, ErrInvalidDeliveryState) {
		t.Errorf("expected ErrInvalidDeliveryState for failed notification, got %v", err)
	}

	// 4. Exhausted attempts
	notifExhausted := &domain.DirectNotification{
		ID:             "notif-exhausted",
		TemplateID:     tpl.ID,
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  5,
	}
	directRepo.notifications[notifExhausted.ID] = notifExhausted

	err = svc.Deliver(context.Background(), notifExhausted.ID)
	if !errors.Is(err, ErrInvalidDeliveryState) {
		t.Errorf("expected ErrInvalidDeliveryState for exhausted attempts notification, got %v", err)
	}

	// 5. Not Found
	err = svc.Deliver(context.Background(), "missing-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing notification, got %v", err)
	}

	// 6. Empty ID
	err = svc.Deliver(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty ID, got %v", err)
	}
}

func TestDeliveryService_Deliver_TemplateIssues(t *testing.T) {
	svc, directRepo, _, templateRepo, _ := setupTestDeliveryService()

	// 1. Inactive Template
	draftTpl := &domain.EmailTemplate{
		ID:       "tpl-draft",
		Subject:  "Draft Subject",
		HTMLBody: "<p>Draft</p>",
		Status:   domain.TemplateStatusDraft,
	}
	templateRepo.templates[draftTpl.ID] = draftTpl

	notifDraft := &domain.DirectNotification{
		ID:             "notif-draft",
		TemplateID:     draftTpl.ID,
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
	}
	directRepo.notifications[notifDraft.ID] = notifDraft

	err := svc.Deliver(context.Background(), notifDraft.ID)
	if !errors.Is(err, ErrTemplateInactive) {
		t.Errorf("expected ErrTemplateInactive, got %v", err)
	}

	// 2. Missing Template Variable
	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-active",
		Subject:  "Hi {{name}}",
		HTMLBody: "<p>Your code is {{code}}</p>",
		Status:   domain.TemplateStatusActive,
	}
	templateRepo.templates[activeTpl.ID] = activeTpl

	payloadWithoutCode, _ := json.Marshal(map[string]any{"name": "Alice"})
	notifMissingVar := &domain.DirectNotification{
		ID:             "notif-missing-var",
		TemplateID:     activeTpl.ID,
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		Payload:        payloadWithoutCode,
	}
	directRepo.notifications[notifMissingVar.ID] = notifMissingVar

	err = svc.Deliver(context.Background(), notifMissingVar.ID)
	if !errors.Is(err, ErrMissingVariable) {
		t.Errorf("expected ErrMissingVariable, got %v", err)
	}
}

func TestDeliveryService_DeliverClaimed_Success(t *testing.T) {
	svc, directRepo, attemptRepo, templateRepo, sender := setupTestDeliveryService()

	tpl := &domain.EmailTemplate{
		ID:            "tpl-claimed-1",
		TemplateKey:   "welcome",
		Name:          "Welcome",
		Subject:       "Hello {{name}}",
		HTMLBody:      "<p>Welcome {{name}}</p>",
		PlainTextBody: "Welcome {{name}}",
		Status:        domain.TemplateStatusActive,
		Version:       1,
	}
	templateRepo.templates[tpl.ID] = tpl

	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{"name": "Bob"})
	notif := &domain.DirectNotification{
		ID:             "notif-claimed-1",
		TemplateID:     tpl.ID,
		RecipientEmail: "bob@example.com",
		DeliveryStatus: domain.DeliveryStatusSending,
		AttemptsCount:  1,
		LastAttemptAt:  &now,
		Payload:        payload,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	directRepo.notifications[notif.ID] = notif

	attempt := &domain.DeliveryAttempt{
		ID:            "attempt-claimed-1",
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      notif.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
		AttemptedAt:   now,
		CreatedAt:     now,
	}
	attemptRepo.attempts[attempt.ID] = attempt

	if err := svc.DeliverClaimed(context.Background(), notif); err != nil {
		t.Fatalf("expected DeliverClaimed success, got: %v", err)
	}

	if notif.DeliveryStatus != domain.DeliveryStatusSent {
		t.Errorf("expected status sent, got %s", notif.DeliveryStatus)
	}
	if attempt.Status != domain.DeliveryStatusSent {
		t.Errorf("expected attempt status sent, got %s", attempt.Status)
	}
	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sender.sentMessages))
	}
}

func TestDeliveryService_DeliverClaimed_InvalidStatus(t *testing.T) {
	svc, _, _, _, _ := setupTestDeliveryService()

	// 1. Nil notification
	if err := svc.DeliverClaimed(context.Background(), nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for nil notification, got %v", err)
	}

	// 2. Notification not in 'sending' state (e.g. 'pending')
	notifPending := &domain.DirectNotification{
		ID:             "notif-p",
		DeliveryStatus: domain.DeliveryStatusPending,
	}
	if err := svc.DeliverClaimed(context.Background(), notifPending); !errors.Is(err, ErrInvalidDeliveryState) {
		t.Errorf("expected ErrInvalidDeliveryState for pending notification in DeliverClaimed, got %v", err)
	}
}

func TestDeliveryService_Audit_Deliver_Success_SystemActor(t *testing.T) {
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	attemptRepo := &mockAttemptRepo{
		attempts: make(map[string]*domain.DeliveryAttempt),
	}
	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-delivery-1": {
				ID:            "tpl-delivery-1",
				TemplateKey:   "alert",
				Name:          "Alert",
				Subject:       "System Alert",
				HTMLBody:      "<p>System Alert Body</p>",
				PlainTextBody: "System Alert Body",
				Status:        domain.TemplateStatusActive,
			},
		},
	}
	sender := &mockSender{}
	auditRepo := &mockActivityLogRepoForDirect{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewDeliveryService(directRepo, attemptRepo, templateRepo, sender, auditSvc)

	notif := &domain.DirectNotification{
		ID:             "notif-1",
		TemplateID:     "tpl-delivery-1",
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
	}
	directRepo.notifications[notif.ID] = notif

	// Deliver with background/system context (no user principal)
	err := svc.Deliver(context.Background(), notif.ID)
	if err != nil {
		t.Fatalf("expected Deliver success, got: %v", err)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected exactly 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventDirectDelivered {
		t.Errorf("expected event type %q, got %q", domain.EventDirectDelivered, log.EventType)
	}
	if log.ActorType != domain.ActorTypeSystem {
		t.Errorf("expected actor type %q for worker/unauthenticated delivery, got %q", domain.ActorTypeSystem, log.ActorType)
	}
	if log.TargetType != domain.TargetTypeDirectNotification {
		t.Errorf("expected target type %q, got %q", domain.TargetTypeDirectNotification, log.TargetType)
	}
	if log.TargetID != notif.ID {
		t.Errorf("expected target ID %q, got %q", notif.ID, log.TargetID)
	}
	if log.Status != domain.ActivityStatusSuccess {
		t.Errorf("expected status success, got %q", log.Status)
	}
	if !strings.Contains(log.Summary, "user@example.com") {
		t.Errorf("expected summary to contain recipient email, got %q", log.Summary)
	}

	var details directDeliveredDetails
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details.NotificationID != notif.ID {
		t.Errorf("expected notificationId %q, got %q", notif.ID, details.NotificationID)
	}
	if details.RecipientEmail != "user@example.com" {
		t.Errorf("expected recipientEmail 'user@example.com', got %q", details.RecipientEmail)
	}
	if details.Subject != "System Alert" {
		t.Errorf("expected subject 'System Alert', got %q", details.Subject)
	}
	if details.AttemptsCount != 1 {
		t.Errorf("expected attemptsCount 1, got %d", details.AttemptsCount)
	}
}

func TestDeliveryService_Audit_Deliver_Success_UserActorFromContext(t *testing.T) {
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	attemptRepo := &mockAttemptRepo{
		attempts: make(map[string]*domain.DeliveryAttempt),
	}
	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-delivery-1": {
				ID:            "tpl-delivery-1",
				TemplateKey:   "alert",
				Name:          "Alert",
				Subject:       "Manual Admin Trigger",
				HTMLBody:      "<p>Manual Body</p>",
				PlainTextBody: "Manual Body",
				Status:        domain.TemplateStatusActive,
			},
		},
	}
	sender := &mockSender{}
	auditRepo := &mockActivityLogRepoForDirect{}
	auditSvc := audit.NewService(auditRepo)
	svc := NewDeliveryService(directRepo, attemptRepo, templateRepo, sender, auditSvc)

	notif := &domain.DirectNotification{
		ID:             "notif-manual-1",
		TemplateID:     "tpl-delivery-1",
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
	}
	directRepo.notifications[notif.ID] = notif

	ctx := authContext("usr-admin-manual", "Admin")
	err := svc.Deliver(ctx, notif.ID)
	if err != nil {
		t.Fatalf("expected Deliver success, got: %v", err)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected exactly 1 audit log recorded, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.ActorType != domain.ActorTypeUser {
		t.Errorf("expected actor type %q for authenticated delivery, got %q", domain.ActorTypeUser, log.ActorType)
	}
	if log.ActorUserID == nil || *log.ActorUserID != "usr-admin-manual" {
		t.Errorf("expected actor user ID 'usr-admin-manual', got %v", log.ActorUserID)
	}
}

func TestDeliveryService_Audit_Deliver_RetryFailure_NoAuditEvent(t *testing.T) {
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	attemptRepo := &mockAttemptRepo{
		attempts: make(map[string]*domain.DeliveryAttempt),
	}
	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-delivery-1": {
				ID:            "tpl-delivery-1",
				Subject:       "Subject",
				HTMLBody:      "<p>Body</p>",
				PlainTextBody: "Body",
				Status:        domain.TemplateStatusActive,
			},
		},
	}
	sender := &mockSender{
		sendErr: errors.New("temporary SMTP connection timeout"),
	}
	auditRepo := &mockActivityLogRepoForDirect{}
	auditSvc := audit.NewService(auditRepo)

	// maxAttempts = 3, this is attempt 1 (notification status returns to pending for retry)
	svc := NewDeliveryServiceWithMaxAttempts(directRepo, attemptRepo, templateRepo, sender, 3, auditSvc)

	notif := &domain.DirectNotification{
		ID:             "notif-retry",
		TemplateID:     "tpl-delivery-1",
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
	}
	directRepo.notifications[notif.ID] = notif

	err := svc.Deliver(context.Background(), notif.ID)
	if err == nil {
		t.Fatal("expected delivery error, got nil")
	}

	// Notification returned to pending
	if notif.DeliveryStatus != domain.DeliveryStatusPending {
		t.Errorf("expected notification status pending for retry, got %s", notif.DeliveryStatus)
	}

	// Crucial check: NO audit event should be generated for intermediate retry attempt!
	if len(auditRepo.recordedLogs) != 0 {
		t.Errorf("expected 0 audit logs for intermediate retry failure, got %d", len(auditRepo.recordedLogs))
	}
}

func TestDeliveryService_Audit_Deliver_FinalFailure_RecordsDirectFailed(t *testing.T) {
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	attemptRepo := &mockAttemptRepo{
		attempts: make(map[string]*domain.DeliveryAttempt),
	}
	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-delivery-1": {
				ID:            "tpl-delivery-1",
				Subject:       "Subject",
				HTMLBody:      "<p>Body</p>",
				PlainTextBody: "Body",
				Status:        domain.TemplateStatusActive,
			},
		},
	}
	sender := &mockSender{
		sendErr: errors.New("550 5.1.1 User unknown"),
	}
	auditRepo := &mockActivityLogRepoForDirect{}
	auditSvc := audit.NewService(auditRepo)

	// maxAttempts = 1 -> attempt 1 fails and reaches final failed state!
	svc := NewDeliveryServiceWithMaxAttempts(directRepo, attemptRepo, templateRepo, sender, 1, auditSvc)

	notif := &domain.DirectNotification{
		ID:             "notif-final-fail",
		TemplateID:     "tpl-delivery-1",
		RecipientEmail: "baduser@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
	}
	directRepo.notifications[notif.ID] = notif

	err := svc.Deliver(context.Background(), notif.ID)
	if err == nil {
		t.Fatal("expected delivery error, got nil")
	}

	if notif.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Errorf("expected notification status failed, got %s", notif.DeliveryStatus)
	}

	if len(auditRepo.recordedLogs) != 1 {
		t.Fatalf("expected exactly 1 audit log recorded for final failure, got %d", len(auditRepo.recordedLogs))
	}

	log := auditRepo.recordedLogs[0]
	if log.EventType != domain.EventDirectFailed {
		t.Errorf("expected event type %q, got %q", domain.EventDirectFailed, log.EventType)
	}
	if log.Status != domain.ActivityStatusFailure {
		t.Errorf("expected status %q, got %q", domain.ActivityStatusFailure, log.Status)
	}
	if log.ErrorMessage == nil || !strings.Contains(*log.ErrorMessage, "550 5.1.1 User unknown") {
		t.Errorf("expected error message to contain 550 5.1.1 User unknown, got %v", log.ErrorMessage)
	}

	var details directFailedDetails
	if err := json.Unmarshal(log.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details.NotificationID != notif.ID {
		t.Errorf("expected notificationId %q, got %q", notif.ID, details.NotificationID)
	}
	if details.RecipientEmail != "baduser@example.com" {
		t.Errorf("expected recipientEmail 'baduser@example.com', got %q", details.RecipientEmail)
	}
	if details.ErrorMessage != "550 5.1.1 User unknown" {
		t.Errorf("expected errorMessage, got %q", details.ErrorMessage)
	}
	if details.DeliveryStatus != string(domain.DeliveryStatusFailed) {
		t.Errorf("expected deliveryStatus failed, got %q", details.DeliveryStatus)
	}
}

func TestDeliveryService_Audit_Deliver_AuditFailure_PropagatesError(t *testing.T) {
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
		attempts:      make(map[string]*domain.DeliveryAttempt),
	}
	attemptRepo := &mockAttemptRepo{
		attempts: make(map[string]*domain.DeliveryAttempt),
	}
	templateRepo := &mockTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-delivery-1": {
				ID:            "tpl-delivery-1",
				Subject:       "Subject",
				HTMLBody:      "<p>Body</p>",
				PlainTextBody: "Body",
				Status:        domain.TemplateStatusActive,
			},
		},
	}
	sender := &mockSender{}
	auditRepo := &mockActivityLogRepoForDirect{
		createErr: errors.New("audit log write failed"),
	}
	auditSvc := audit.NewService(auditRepo)
	svc := NewDeliveryService(directRepo, attemptRepo, templateRepo, sender, auditSvc)

	notif := &domain.DirectNotification{
		ID:             "notif-audit-fail",
		TemplateID:     "tpl-delivery-1",
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
	}
	directRepo.notifications[notif.ID] = notif

	err := svc.Deliver(context.Background(), notif.ID)
	if err == nil {
		t.Fatal("expected error when audit recording fails, got nil")
	}
	if !strings.Contains(err.Error(), "audit log write failed") {
		t.Errorf("expected audit error to be propagated, got %v", err)
	}
}
