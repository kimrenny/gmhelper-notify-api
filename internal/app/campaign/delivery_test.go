package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
)

type mockTemplateRepo struct {
	mu        sync.Mutex
	templates map[string]*domain.EmailTemplate
}

func newMockTemplateRepo() *mockTemplateRepo {
	return &mockTemplateRepo{templates: make(map[string]*domain.EmailTemplate)}
}

func (m *mockTemplateRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.templates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return t, nil
}

func (m *mockTemplateRepo) GetByKey(ctx context.Context, key string) (*domain.EmailTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.templates {
		if t.TemplateKey == key {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockTemplateRepo) Create(ctx context.Context, template *domain.EmailTemplate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.templates[template.ID] = template
	return nil
}

func (m *mockTemplateRepo) Update(ctx context.Context, template *domain.EmailTemplate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.templates[template.ID] = template
	return nil
}

func (m *mockTemplateRepo) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.templates, id)
	return nil
}

func (m *mockTemplateRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*domain.EmailTemplate
	for _, t := range m.templates {
		list = append(list, t)
	}
	return list, nil
}

type mockAttemptRepo struct {
	mu       sync.Mutex
	attempts map[string]*domain.DeliveryAttempt
}

func newMockAttemptRepo() *mockAttemptRepo {
	return &mockAttemptRepo{attempts: make(map[string]*domain.DeliveryAttempt)}
}

func (m *mockAttemptRepo) GetByID(ctx context.Context, id string) (*domain.DeliveryAttempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.attempts[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return a, nil
}

func (m *mockAttemptRepo) Create(ctx context.Context, attempt *domain.DeliveryAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts[attempt.ID] = attempt
	return nil
}

func (m *mockAttemptRepo) Update(ctx context.Context, attempt *domain.DeliveryAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts[attempt.ID] = attempt
	return nil
}

func (m *mockAttemptRepo) ListByTarget(ctx context.Context, targetType domain.DeliveryTargetType, targetID string) ([]*domain.DeliveryAttempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*domain.DeliveryAttempt
	for _, a := range m.attempts {
		if a.TargetType == targetType && a.TargetID == targetID {
			list = append(list, a)
		}
	}
	return list, nil
}

type mockSender struct {
	mu      sync.Mutex
	sent    []*email.Message
	sendErr error
}

func (m *mockSender) Send(ctx context.Context, msg *email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, msg)
	return nil
}

type mockUserResolver struct {
	getUserFunc func(ctx context.Context, userID string) (*userclient.User, error)
}

func (m *mockUserResolver) GetUserByID(ctx context.Context, userID string) (*userclient.User, error) {
	if m.getUserFunc != nil {
		return m.getUserFunc(ctx, userID)
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserResolver) SearchUsers(ctx context.Context, query string, limit int) ([]userclient.User, error) {
	return nil, nil
}

func TestDeliverClaimed_NilRecipient(t *testing.T) {
	svc := NewDeliveryService(nil, nil, nil, nil, nil, nil, nil)
	err := svc.DeliverClaimed(context.Background(), nil)
	if !errors.Is(err, ErrRecipientNil) {
		t.Fatalf("expected ErrRecipientNil, got %v", err)
	}
}

func TestDeliverClaimed_Success(t *testing.T) {
	ctx := context.Background()
	cmpID := "cmp-100"
	tplID := "tpl-100"
	recID := "rec-100"

	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			cmpID: {ID: cmpID, TemplateID: tplID, Status: domain.CampaignStatusRunning},
		},
	}
	rRepo := newMockRecipientRepo()
	recipient := &domain.CampaignRecipient{
		ID:             recID,
		CampaignID:     cmpID,
		ExternalUserID: "user-42",
		RecipientEmail: "alice@example.com",
		RecipientName:  "Alice",
		DeliveryStatus: domain.DeliveryStatusSending,
		AttemptsCount:  1,
	}
	rRepo.recipients[recID] = recipient

	tRepo := newMockTemplateRepo()
	tRepo.templates[tplID] = &domain.EmailTemplate{
		ID:            tplID,
		TemplateKey:   "monthly-digest",
		Subject:       "Hello {{username}}",
		HTMLBody:      "<p>Welcome {{username}} (role: {{role}})</p>",
		PlainTextBody: "Welcome {{username}} (role: {{role}})",
		Status:        domain.TemplateStatusActive,
	}

	aRepo := newMockAttemptRepo()
	sender := &mockSender{}
	uResolver := &mockUserResolver{
		getUserFunc: func(ctx context.Context, userID string) (*userclient.User, error) {
			return &userclient.User{
				ID:       userID,
				Username: "AliceInWonderland",
				Email:    "alice@example.com",
				Role:     "Admin",
			}, nil
		},
	}

	svc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, sender, uResolver, nil)

	err := svc.DeliverClaimed(ctx, recipient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Verify recipient updated to sent
	if recipient.DeliveryStatus != domain.DeliveryStatusSent {
		t.Fatalf("expected recipient status sent, got %s", recipient.DeliveryStatus)
	}
	if recipient.SentAt == nil {
		t.Fatal("expected sent_at to be populated")
	}

	// 2. Verify DeliveryAttempt recorded and marked sent
	if len(aRepo.attempts) != 1 {
		t.Fatalf("expected 1 delivery attempt, got %d", len(aRepo.attempts))
	}
	var attempt *domain.DeliveryAttempt
	for _, a := range aRepo.attempts {
		attempt = a
	}
	if attempt.TargetType != domain.DeliveryTargetCampaignRecipient || attempt.TargetID != recID {
		t.Fatalf("unexpected attempt target: %+v", attempt)
	}
	if attempt.Status != domain.DeliveryStatusSent {
		t.Fatalf("expected attempt status sent, got %s", attempt.Status)
	}

	// 3. Verify email sent via sender
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(sender.sent))
	}
	sentMsg := sender.sent[0]
	if sentMsg.To != "alice@example.com" {
		t.Fatalf("expected To 'alice@example.com', got '%s'", sentMsg.To)
	}
	if sentMsg.Subject != "Hello AliceInWonderland" {
		t.Fatalf("expected Subject 'Hello AliceInWonderland', got '%s'", sentMsg.Subject)
	}
	if !strings.Contains(sentMsg.HTMLBody, "role: Admin") {
		t.Fatalf("expected HTML body to contain role: Admin, got: %s", sentMsg.HTMLBody)
	}
}

func TestDeliverClaimed_CampaignLoadFailure(t *testing.T) {
	ctx := context.Background()
	recID := "rec-err-cmp"
	recipient := &domain.CampaignRecipient{
		ID:             recID,
		CampaignID:     "missing-cmp",
		RecipientEmail: "bob@example.com",
		DeliveryStatus: domain.DeliveryStatusSending,
		AttemptsCount:  1,
	}

	cRepo := &mockRepo{campaigns: make(map[string]*domain.NotificationCampaign)}
	rRepo := newMockRecipientRepo()
	rRepo.recipients[recID] = recipient
	tRepo := newMockTemplateRepo()
	aRepo := newMockAttemptRepo()
	sender := &mockSender{}

	svc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, sender, nil, nil)

	err := svc.DeliverClaimed(ctx, recipient)
	if err == nil {
		t.Fatal("expected error for missing campaign, got nil")
	}

	if recipient.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Fatalf("expected recipient status failed, got %s", recipient.DeliveryStatus)
	}
	if len(aRepo.attempts) != 1 {
		t.Fatalf("expected 1 failed attempt recorded, got %d", len(aRepo.attempts))
	}
}

func TestDeliverClaimed_TemplateInactive(t *testing.T) {
	ctx := context.Background()
	cmpID := "cmp-inact"
	tplID := "tpl-inact"
	recID := "rec-inact"

	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			cmpID: {ID: cmpID, TemplateID: tplID, Status: domain.CampaignStatusRunning},
		},
	}
	tRepo := newMockTemplateRepo()
	tRepo.templates[tplID] = &domain.EmailTemplate{
		ID:     tplID,
		Status: domain.TemplateStatusDraft, // Inactive!
	}
	rRepo := newMockRecipientRepo()
	recipient := &domain.CampaignRecipient{
		ID:             recID,
		CampaignID:     cmpID,
		RecipientEmail: "carol@example.com",
		DeliveryStatus: domain.DeliveryStatusSending,
		AttemptsCount:  1,
	}
	rRepo.recipients[recID] = recipient
	aRepo := newMockAttemptRepo()
	sender := &mockSender{}

	svc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, sender, nil, nil)

	err := svc.DeliverClaimed(ctx, recipient)
	if !errors.Is(err, ErrTemplateInactive) {
		t.Fatalf("expected ErrTemplateInactive, got %v", err)
	}

	if recipient.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Fatalf("expected recipient status failed, got %s", recipient.DeliveryStatus)
	}
}

func TestDeliverClaimed_UserResolverError_FallbacksGracefully(t *testing.T) {
	ctx := context.Background()
	cmpID := "cmp-fallback"
	tplID := "tpl-fallback"
	recID := "rec-fallback"

	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			cmpID: {ID: cmpID, TemplateID: tplID, Status: domain.CampaignStatusRunning},
		},
	}
	tRepo := newMockTemplateRepo()
	tRepo.templates[tplID] = &domain.EmailTemplate{
		ID:            tplID,
		TemplateKey:   "fallback-tpl",
		Subject:       "Hello {{username}}",
		HTMLBody:      "<p>Email: {{email}}</p>",
		PlainTextBody: "Email: {{email}}",
		Status:        domain.TemplateStatusActive,
	}
	rRepo := newMockRecipientRepo()
	recipient := &domain.CampaignRecipient{
		ID:             recID,
		CampaignID:     cmpID,
		ExternalUserID: "user-404",
		RecipientEmail: "dave@example.com",
		RecipientName:  "Dave",
		DeliveryStatus: domain.DeliveryStatusSending,
		AttemptsCount:  1,
	}
	rRepo.recipients[recID] = recipient
	aRepo := newMockAttemptRepo()
	sender := &mockSender{}
	uResolver := &mockUserResolver{
		getUserFunc: func(ctx context.Context, userID string) (*userclient.User, error) {
			return nil, errors.New("user service unavailable")
		},
	}

	svc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, sender, uResolver, nil)

	err := svc.DeliverClaimed(ctx, recipient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recipient.DeliveryStatus != domain.DeliveryStatusSent {
		t.Fatalf("expected sent, got %s", recipient.DeliveryStatus)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected email sent, got %d", len(sender.sent))
	}
	if sender.sent[0].Subject != "Hello Dave" {
		t.Fatalf("expected fallback username Dave, got %s", sender.sent[0].Subject)
	}
}

func TestDeliverClaimed_RenderError(t *testing.T) {
	ctx := context.Background()
	cmpID := "cmp-render-err"
	tplID := "tpl-render-err"
	recID := "rec-render-err"

	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			cmpID: {ID: cmpID, TemplateID: tplID, Status: domain.CampaignStatusRunning},
		},
	}
	tRepo := newMockTemplateRepo()
	tRepo.templates[tplID] = &domain.EmailTemplate{
		ID:            tplID,
		TemplateKey:   "render-err-tpl",
		Subject:       "Hello {{missing_custom_var}}",
		HTMLBody:      "<p>Error</p>",
		PlainTextBody: "Error",
		Status:        domain.TemplateStatusActive,
	}
	rRepo := newMockRecipientRepo()
	recipient := &domain.CampaignRecipient{
		ID:             recID,
		CampaignID:     cmpID,
		RecipientEmail: "eve@example.com",
		DeliveryStatus: domain.DeliveryStatusSending,
		AttemptsCount:  1,
	}
	rRepo.recipients[recID] = recipient
	aRepo := newMockAttemptRepo()
	sender := &mockSender{}

	svc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, sender, nil, nil)

	err := svc.DeliverClaimed(ctx, recipient)
	if err == nil {
		t.Fatal("expected rendering error, got nil")
	}

	if recipient.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Fatalf("expected failed, got %s", recipient.DeliveryStatus)
	}
}

func TestDeliverClaimed_SMTPSendFailure(t *testing.T) {
	ctx := context.Background()
	cmpID := "cmp-smtp-err"
	tplID := "tpl-smtp-err"
	recID := "rec-smtp-err"

	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			cmpID: {ID: cmpID, TemplateID: tplID, Status: domain.CampaignStatusRunning},
		},
	}
	tRepo := newMockTemplateRepo()
	tRepo.templates[tplID] = &domain.EmailTemplate{
		ID:            tplID,
		Subject:       "Hello",
		HTMLBody:      "<p>Hello</p>",
		PlainTextBody: "Hello",
		Status:        domain.TemplateStatusActive,
	}
	rRepo := newMockRecipientRepo()
	recipient := &domain.CampaignRecipient{
		ID:             recID,
		CampaignID:     cmpID,
		RecipientEmail: "frank@example.com",
		DeliveryStatus: domain.DeliveryStatusSending,
		AttemptsCount:  1,
	}
	rRepo.recipients[recID] = recipient
	aRepo := newMockAttemptRepo()
	sender := &mockSender{
		sendErr: errors.New("smtp connection refused"),
	}

	svc := NewDeliveryService(cRepo, rRepo, tRepo, aRepo, sender, nil, nil)

	err := svc.DeliverClaimed(ctx, recipient)
	if err == nil {
		t.Fatal("expected SMTP send error, got nil")
	}

	if recipient.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Fatalf("expected recipient status failed, got %s", recipient.DeliveryStatus)
	}
	if recipient.ErrorMessage != "smtp connection refused" {
		t.Fatalf("unexpected recipient error message: %s", recipient.ErrorMessage)
	}

	if len(aRepo.attempts) != 1 {
		t.Fatalf("expected 1 attempt recorded, got %d", len(aRepo.attempts))
	}
	var attempt *domain.DeliveryAttempt
	for _, a := range aRepo.attempts {
		attempt = a
	}
	if attempt.Status != domain.DeliveryStatusFailed {
		t.Fatalf("expected attempt status failed, got %s", attempt.Status)
	}
	if attempt.ErrorMessage != "smtp connection refused" {
		t.Fatalf("unexpected attempt error message: %s", attempt.ErrorMessage)
	}
}

func TestFinalizeCampaignIfDone(t *testing.T) {
	ctx := context.Background()

	// 1. Still has pending or sending recipients -> do not finalize
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"c1": {ID: "c1", Status: domain.CampaignStatusRunning},
		},
	}
	rRepo := newMockRecipientRepo()
	rRepo.recipients["r1"] = &domain.CampaignRecipient{ID: "r1", CampaignID: "c1", DeliveryStatus: domain.DeliveryStatusPending}
	rRepo.recipients["r2"] = &domain.CampaignRecipient{ID: "r2", CampaignID: "c1", DeliveryStatus: domain.DeliveryStatusSent}

	svc := NewDeliveryService(cRepo, rRepo, nil, nil, nil, nil, nil)

	finalized, status, err := svc.FinalizeCampaignIfDone(ctx, "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalized {
		t.Fatal("expected finalized=false when pending recipients exist")
	}
	if status != domain.CampaignStatusRunning {
		t.Fatalf("expected running status, got %s", status)
	}
	if cRepo.campaigns["c1"].Status != domain.CampaignStatusRunning {
		t.Fatalf("expected campaign status running, got %s", cRepo.campaigns["c1"].Status)
	}

	// 2. All recipients sent -> completed
	rRepo.recipients["r1"].DeliveryStatus = domain.DeliveryStatusSent
	finalized, status, err = svc.FinalizeCampaignIfDone(ctx, "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !finalized {
		t.Fatal("expected finalized=true when all sent")
	}
	if status != domain.CampaignStatusCompleted {
		t.Fatalf("expected completed status, got %s", status)
	}
	if cRepo.campaigns["c1"].Status != domain.CampaignStatusCompleted {
		t.Fatalf("expected campaign status completed, got %s", cRepo.campaigns["c1"].Status)
	}
	if cRepo.campaigns["c1"].CompletedAt == nil {
		t.Fatal("expected completed_at to be set on campaign")
	}

	// 3. Mixed success and failure -> partially_failed
	cRepo.campaigns["c2"] = &domain.NotificationCampaign{ID: "c2", Status: domain.CampaignStatusRunning}
	rRepo.recipients["r3"] = &domain.CampaignRecipient{ID: "r3", CampaignID: "c2", DeliveryStatus: domain.DeliveryStatusSent}
	rRepo.recipients["r4"] = &domain.CampaignRecipient{ID: "r4", CampaignID: "c2", DeliveryStatus: domain.DeliveryStatusFailed}

	finalized, status, err = svc.FinalizeCampaignIfDone(ctx, "c2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !finalized || status != domain.CampaignStatusPartiallyFailed {
		t.Fatalf("expected partially_failed, got finalized=%v status=%s", finalized, status)
	}
	if cRepo.campaigns["c2"].Status != domain.CampaignStatusPartiallyFailed {
		t.Fatalf("expected campaign status partially_failed, got %s", cRepo.campaigns["c2"].Status)
	}

	// 4. All failed -> failed
	cRepo.campaigns["c3"] = &domain.NotificationCampaign{ID: "c3", Status: domain.CampaignStatusRunning}
	rRepo.recipients["r5"] = &domain.CampaignRecipient{ID: "r5", CampaignID: "c3", DeliveryStatus: domain.DeliveryStatusFailed}

	finalized, status, err = svc.FinalizeCampaignIfDone(ctx, "c3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !finalized || status != domain.CampaignStatusFailed {
		t.Fatalf("expected failed, got finalized=%v status=%s", finalized, status)
	}
	if cRepo.campaigns["c3"].Status != domain.CampaignStatusFailed {
		t.Fatalf("expected campaign status failed, got %s", cRepo.campaigns["c3"].Status)
	}

	// 5. Zero recipients in DB -> do not finalize (population in progress or managed by populator)
	cRepo.campaigns["c4"] = &domain.NotificationCampaign{ID: "c4", Status: domain.CampaignStatusRunning}
	finalized, status, err = svc.FinalizeCampaignIfDone(ctx, "c4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalized {
		t.Fatal("expected finalized=false when 0 recipients exist (population in progress)")
	}
	if status != domain.CampaignStatusRunning {
		t.Fatalf("expected running status for 0 recipients, got %s", status)
	}
	if cRepo.campaigns["c4"].Status != domain.CampaignStatusRunning {
		t.Fatalf("expected campaign status running, got %s", cRepo.campaigns["c4"].Status)
	}
}

func TestDeliveryService_Audit_Finalize_Completed(t *testing.T) {
	ctx := context.Background()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"camp-fin-1": {ID: "camp-fin-1", Name: "Spring Promo", Status: domain.CampaignStatusRunning},
		},
	}
	rRepo := newMockRecipientRepo()
	rRepo.recipients["r1"] = &domain.CampaignRecipient{ID: "r1", CampaignID: "camp-fin-1", DeliveryStatus: domain.DeliveryStatusSent}
	rRepo.recipients["r2"] = &domain.CampaignRecipient{ID: "r2", CampaignID: "camp-fin-1", DeliveryStatus: domain.DeliveryStatusSent}

	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)

	svc := NewDeliveryService(cRepo, rRepo, nil, nil, nil, nil, auditSvc)

	finalized, status, err := svc.FinalizeCampaignIfDone(ctx, "camp-fin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !finalized || status != domain.CampaignStatusCompleted {
		t.Fatalf("expected finalized completed, got %v %s", finalized, status)
	}

	if len(auditRepo.logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(auditRepo.logs))
	}
	entry := auditRepo.logs[0]
	if entry.EventType != domain.EventCampaignCompleted {
		t.Errorf("expected event %s, got %s", domain.EventCampaignCompleted, entry.EventType)
	}
	if entry.ActorType != domain.ActorTypeSystem {
		t.Errorf("expected actor type system, got %s", entry.ActorType)
	}
	if entry.TargetType != domain.TargetTypeCampaign {
		t.Errorf("expected target type campaign, got %s", entry.TargetType)
	}
	if entry.TargetID != "camp-fin-1" {
		t.Errorf("expected target ID camp-fin-1, got %s", entry.TargetID)
	}
	if entry.TargetName == nil || *entry.TargetName != "Spring Promo" {
		t.Errorf("expected target name 'Spring Promo', got %v", entry.TargetName)
	}
	if entry.Status != domain.ActivityStatusSuccess {
		t.Errorf("expected status success, got %s", entry.Status)
	}

	var details map[string]interface{}
	if err := json.Unmarshal(entry.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["totalRecipients"] != float64(2) {
		t.Errorf("expected 2 total recipients, got %v", details["totalRecipients"])
	}
	if details["sentCount"] != float64(2) {
		t.Errorf("expected 2 sent, got %v", details["sentCount"])
	}
	if details["failedCount"] != float64(0) {
		t.Errorf("expected 0 failed, got %v", details["failedCount"])
	}
}

func TestDeliveryService_Audit_Finalize_PartiallyFailed(t *testing.T) {
	ctx := context.Background()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"camp-fin-partial": {ID: "camp-fin-partial", Name: "Partial Promo", Status: domain.CampaignStatusRunning},
		},
	}
	rRepo := newMockRecipientRepo()
	rRepo.recipients["r1"] = &domain.CampaignRecipient{ID: "r1", CampaignID: "camp-fin-partial", DeliveryStatus: domain.DeliveryStatusSent}
	rRepo.recipients["r2"] = &domain.CampaignRecipient{ID: "r2", CampaignID: "camp-fin-partial", DeliveryStatus: domain.DeliveryStatusFailed}

	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)

	svc := NewDeliveryService(cRepo, rRepo, nil, nil, nil, nil, auditSvc)

	finalized, status, err := svc.FinalizeCampaignIfDone(ctx, "camp-fin-partial")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !finalized || status != domain.CampaignStatusPartiallyFailed {
		t.Fatalf("expected finalized partially_failed, got %v %s", finalized, status)
	}

	if len(auditRepo.logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(auditRepo.logs))
	}
	entry := auditRepo.logs[0]
	if entry.EventType != domain.EventCampaignCompleted {
		t.Errorf("expected event %s, got %s", domain.EventCampaignCompleted, entry.EventType)
	}
	if entry.Status != domain.ActivityStatusWarning {
		t.Errorf("expected status warning for partially failed, got %s", entry.Status)
	}

	var details map[string]interface{}
	if err := json.Unmarshal(entry.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["sentCount"] != float64(1) || details["failedCount"] != float64(1) {
		t.Errorf("unexpected counts: %v", details)
	}
}

func TestDeliveryService_Audit_Finalize_Failed(t *testing.T) {
	ctx := context.Background()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"camp-fin-fail": {ID: "camp-fin-fail", Name: "Failed Promo", Status: domain.CampaignStatusRunning},
		},
	}
	rRepo := newMockRecipientRepo()
	rRepo.recipients["r1"] = &domain.CampaignRecipient{ID: "r1", CampaignID: "camp-fin-fail", DeliveryStatus: domain.DeliveryStatusFailed}
	rRepo.recipients["r2"] = &domain.CampaignRecipient{ID: "r2", CampaignID: "camp-fin-fail", DeliveryStatus: domain.DeliveryStatusFailed}

	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)

	svc := NewDeliveryService(cRepo, rRepo, nil, nil, nil, nil, auditSvc)

	finalized, status, err := svc.FinalizeCampaignIfDone(ctx, "camp-fin-fail")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !finalized || status != domain.CampaignStatusFailed {
		t.Fatalf("expected finalized failed, got %v %s", finalized, status)
	}

	if len(auditRepo.logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(auditRepo.logs))
	}
	entry := auditRepo.logs[0]
	if entry.EventType != domain.EventCampaignFailed {
		t.Errorf("expected event %s, got %s", domain.EventCampaignFailed, entry.EventType)
	}
	if entry.Status != domain.ActivityStatusFailure {
		t.Errorf("expected status failure, got %s", entry.Status)
	}
	if entry.ErrorMessage == nil || *entry.ErrorMessage == "" {
		t.Errorf("expected non-empty error message, got %v", entry.ErrorMessage)
	}

	var details map[string]interface{}
	if err := json.Unmarshal(entry.Details, &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if details["failedCount"] != float64(2) {
		t.Errorf("expected 2 failed, got %v", details["failedCount"])
	}
}

func TestDeliveryService_Audit_Finalize_AuditFailure_DoesNotFailCampaign(t *testing.T) {
	ctx := context.Background()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"camp-fin-audit-err": {ID: "camp-fin-audit-err", Name: "Fin Audit Err", Status: domain.CampaignStatusRunning},
		},
	}
	rRepo := newMockRecipientRepo()
	rRepo.recipients["r1"] = &domain.CampaignRecipient{ID: "r1", CampaignID: "camp-fin-audit-err", DeliveryStatus: domain.DeliveryStatusSent}

	auditRepo := &mockActivityLogRepoForCampaign{createErr: errors.New("audit db failure")}
	auditSvc := audit.NewService(auditRepo)

	svc := NewDeliveryService(cRepo, rRepo, nil, nil, nil, nil, auditSvc)

	finalized, status, err := svc.FinalizeCampaignIfDone(ctx, "camp-fin-audit-err")
	if err != nil {
		t.Fatalf("unexpected error finalizing campaign when audit fails: %v", err)
	}
	if !finalized || status != domain.CampaignStatusCompleted {
		t.Fatalf("expected campaign to finalize to completed despite audit failure, got %v %s", finalized, status)
	}
	if cRepo.campaigns["camp-fin-audit-err"].Status != domain.CampaignStatusCompleted {
		t.Errorf("expected campaign in repo to be completed, got %s", cRepo.campaigns["camp-fin-audit-err"].Status)
	}
}

func TestDeliveryService_FinalizeCampaignIfDone_AlreadyTerminal_DoesNotEmitDuplicateAudit(t *testing.T) {
	ctx := context.Background()
	completedAt := time.Now().UTC()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"camp-already-done": {
				ID:          "camp-already-done",
				Name:        "Already Completed",
				Status:      domain.CampaignStatusCompleted,
				CompletedAt: &completedAt,
			},
		},
	}
	rRepo := newMockRecipientRepo()
	rRepo.recipients["r1"] = &domain.CampaignRecipient{ID: "r1", CampaignID: "camp-already-done", DeliveryStatus: domain.DeliveryStatusSent}

	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)

	svc := NewDeliveryService(cRepo, rRepo, nil, nil, nil, nil, auditSvc)

	// Call Finalize on an already completed campaign
	finalized, status, err := svc.FinalizeCampaignIfDone(ctx, "camp-already-done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalized {
		t.Fatal("expected finalized=false on already completed campaign")
	}
	if status != domain.CampaignStatusCompleted {
		t.Fatalf("expected completed status, got %s", status)
	}
	// Verify NO new audit log emitted
	if len(auditRepo.logs) != 0 {
		t.Fatalf("expected 0 audit logs for already completed campaign, got %d", len(auditRepo.logs))
	}
}

func TestDeliveryService_FinalizeCampaignIfDone_CancelledCampaign_DoesNotCompleteOrEmitAudit(t *testing.T) {
	ctx := context.Background()
	cRepo := &mockRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"camp-cancelled": {
				ID:     "camp-cancelled",
				Name:   "Cancelled Campaign",
				Status: domain.CampaignStatusCancelled,
			},
		},
	}
	rRepo := newMockRecipientRepo()
	rRepo.recipients["r1"] = &domain.CampaignRecipient{ID: "r1", CampaignID: "camp-cancelled", DeliveryStatus: domain.DeliveryStatusSent}

	auditRepo := &mockActivityLogRepoForCampaign{}
	auditSvc := audit.NewService(auditRepo)

	svc := NewDeliveryService(cRepo, rRepo, nil, nil, nil, nil, auditSvc)

	finalized, status, err := svc.FinalizeCampaignIfDone(ctx, "camp-cancelled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalized {
		t.Fatal("expected finalized=false on cancelled campaign")
	}
	if status != domain.CampaignStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", status)
	}
	if cRepo.campaigns["camp-cancelled"].Status != domain.CampaignStatusCancelled {
		t.Fatalf("expected campaign status to remain cancelled, got %s", cRepo.campaigns["camp-cancelled"].Status)
	}
	// Verify NO audit log emitted
	if len(auditRepo.logs) != 0 {
		t.Fatalf("expected 0 audit logs on cancelled campaign, got %d", len(auditRepo.logs))
	}
}
