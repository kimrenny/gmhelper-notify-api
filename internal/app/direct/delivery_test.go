package direct

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

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

	svc := NewDeliveryService(directRepo, attemptRepo, templateRepo, sender)
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

	// Verify Notification status marked as failed with error message
	if notif.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Errorf("expected notification status failed, got %s", notif.DeliveryStatus)
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

	// 3. Not Found
	err = svc.Deliver(context.Background(), "missing-id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing notification, got %v", err)
	}

	// 4. Empty ID
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
