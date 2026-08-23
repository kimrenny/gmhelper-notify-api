package direct

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type workerMockRepo struct {
	mu            sync.Mutex
	notifications map[string]*domain.DirectNotification
	listErr       error
}

func (m *workerMockRepo) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.notifications[id]; ok {
		return n, nil
	}
	return nil, domain.ErrNotFound
}

func (m *workerMockRepo) Create(ctx context.Context, n *domain.DirectNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications[n.ID] = n
	return nil
}

func (m *workerMockRepo) CreateWithInitialAttempt(ctx context.Context, n *domain.DirectNotification, a *domain.DeliveryAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications[n.ID] = n
	return nil
}

func (m *workerMockRepo) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []*domain.DirectNotification
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusPending {
			res = append(res, n)
		}
	}
	return res, nil
}

func (m *workerMockRepo) ClaimPending(ctx context.Context, limit int, maxAttempts int) ([]*domain.DirectNotification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []*domain.DirectNotification
	now := time.Now().UTC()
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusPending && (maxAttempts <= 0 || n.AttemptsCount < maxAttempts) {
			n.DeliveryStatus = domain.DeliveryStatusSending
			n.AttemptsCount++
			n.LastAttemptAt = &now
			res = append(res, n)
			if limit > 0 && len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

func (m *workerMockRepo) RecoverStaleSending(ctx context.Context, olderThan time.Duration, maxAttempts int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return 0, m.listErr
	}
	var count int64
	cutoff := time.Now().UTC().Add(-olderThan)
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusSending && n.LastAttemptAt != nil && n.LastAttemptAt.Before(cutoff) {
			if maxAttempts > 0 && n.AttemptsCount >= maxAttempts {
				n.DeliveryStatus = domain.DeliveryStatusFailed
				n.ErrorMessage = "delivery attempt timed out and max attempts reached"
			} else {
				n.DeliveryStatus = domain.DeliveryStatusPending
				n.ErrorMessage = "delivery claim timed out and was recovered"
			}
			count++
		}
	}
	return count, nil
}

func (m *workerMockRepo) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.notifications[id]; ok {
		n.DeliveryStatus = status
		n.AttemptsCount = attempts
		n.LastAttemptAt = lastAttemptAt
		n.SentAt = sentAt
		n.ErrorMessage = errMsg
		return nil
	}
	return domain.ErrNotFound
}

type workerMockSender struct {
	mu       sync.Mutex
	sentMsgs []*email.Message
	failIDs  map[string]bool
}

func (m *workerMockSender) Send(ctx context.Context, msg *email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failIDs != nil && m.failIDs[msg.To] {
		return errors.New("smtp delivery rejected")
	}
	m.sentMsgs = append(m.sentMsgs, msg)
	return nil
}

func setupWorkerTest() (*Worker, *workerMockRepo, *workerMockAttemptRepo, *workerMockTplRepo, *workerMockSender) {
	log, _ := logger.NewLogger("info")
	directRepo := &workerMockRepo{notifications: make(map[string]*domain.DirectNotification)}
	attemptRepo := &workerMockAttemptRepo{attempts: make(map[string]*domain.DeliveryAttempt)}
	tplRepo := &workerMockTplRepo{templates: make(map[string]*domain.EmailTemplate)}
	sender := &workerMockSender{failIDs: make(map[string]bool)}

	deliveryService := NewDeliveryService(directRepo, attemptRepo, tplRepo, sender)
	worker := NewWorker(directRepo, deliveryService, 50*time.Millisecond, 5*time.Minute, 5, log)

	return worker, directRepo, attemptRepo, tplRepo, sender
}

type workerMockAttemptRepo struct {
	mu       sync.Mutex
	attempts map[string]*domain.DeliveryAttempt
}

func (m *workerMockAttemptRepo) GetByID(ctx context.Context, id string) (*domain.DeliveryAttempt, error) {
	return nil, domain.ErrNotFound
}
func (m *workerMockAttemptRepo) Create(ctx context.Context, a *domain.DeliveryAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts[a.ID] = a
	return nil
}
func (m *workerMockAttemptRepo) Update(ctx context.Context, a *domain.DeliveryAttempt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts[a.ID] = a
	return nil
}
func (m *workerMockAttemptRepo) ListByTarget(ctx context.Context, targetType domain.DeliveryTargetType, targetID string) ([]*domain.DeliveryAttempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []*domain.DeliveryAttempt
	for _, a := range m.attempts {
		if a.TargetID == targetID {
			res = append(res, a)
		}
	}
	return res, nil
}

type workerMockTplRepo struct {
	mu        sync.Mutex
	templates map[string]*domain.EmailTemplate
}

func (m *workerMockTplRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.templates[id]; ok {
		return t, nil
	}
	return nil, domain.ErrNotFound
}
func (m *workerMockTplRepo) GetByKey(ctx context.Context, key string) (*domain.EmailTemplate, error) {
	return nil, domain.ErrNotFound
}
func (m *workerMockTplRepo) Create(ctx context.Context, t *domain.EmailTemplate) error {
	return nil
}
func (m *workerMockTplRepo) Update(ctx context.Context, t *domain.EmailTemplate) error {
	return nil
}
func (m *workerMockTplRepo) Delete(ctx context.Context, id string) error {
	return nil
}
func (m *workerMockTplRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

func TestWorker_ProcessPending_Success(t *testing.T) {
	worker, directRepo, attemptRepo, tplRepo, sender := setupWorkerTest()

	tpl := &domain.EmailTemplate{
		ID:       "tpl-1",
		Subject:  "Hello {{name}}",
		HTMLBody: "<p>Hello {{name}}</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	n1 := &domain.DirectNotification{
		ID:               "notif-1",
		TemplateID:       tpl.ID,
		RecipientEmail:   "user1@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
		Payload:          []byte(`{"name":"User 1"}`),
	}
	n2 := &domain.DirectNotification{
		ID:               "notif-2",
		TemplateID:       tpl.ID,
		RecipientEmail:   "user2@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
		Payload:          []byte(`{"name":"User 2"}`),
	}
	directRepo.notifications[n1.ID] = n1
	directRepo.notifications[n2.ID] = n2

	attemptRepo.attempts["att-1"] = &domain.DeliveryAttempt{
		ID:       "att-1",
		TargetID: n1.ID,
		Status:   domain.DeliveryStatusPending,
	}
	attemptRepo.attempts["att-2"] = &domain.DeliveryAttempt{
		ID:       "att-2",
		TargetID: n2.ID,
		Status:   domain.DeliveryStatusPending,
	}

	worker.ProcessPending(context.Background())

	if n1.DeliveryStatus != domain.DeliveryStatusSent {
		t.Errorf("expected notif-1 to be sent, got %s", n1.DeliveryStatus)
	}
	if n2.DeliveryStatus != domain.DeliveryStatusSent {
		t.Errorf("expected notif-2 to be sent, got %s", n2.DeliveryStatus)
	}
	if len(sender.sentMsgs) != 2 {
		t.Errorf("expected 2 emails sent, got %d", len(sender.sentMsgs))
	}
}

func TestWorker_ProcessPending_FailureDoesNotStopCycle(t *testing.T) {
	worker, directRepo, _, tplRepo, sender := setupWorkerTest()

	tpl := &domain.EmailTemplate{
		ID:       "tpl-fail-test",
		Subject:  "Hi",
		HTMLBody: "<p>Hi</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	// n1 will fail in SMTP
	n1 := &domain.DirectNotification{
		ID:               "notif-fail",
		TemplateID:       tpl.ID,
		RecipientEmail:   "fail@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
	}
	// n2 will succeed
	n2 := &domain.DirectNotification{
		ID:               "notif-ok",
		TemplateID:       tpl.ID,
		RecipientEmail:   "ok@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
	}
	directRepo.notifications[n1.ID] = n1
	directRepo.notifications[n2.ID] = n2
	sender.failIDs["fail@example.com"] = true

	worker.ProcessPending(context.Background())

	if n1.DeliveryStatus != domain.DeliveryStatusPending {
		t.Errorf("expected notif-fail to return to pending for retry, got %s", n1.DeliveryStatus)
	}
	if n1.AttemptsCount != 1 {
		t.Errorf("expected notif-fail attempts_count 1, got %d", n1.AttemptsCount)
	}
	if n2.DeliveryStatus != domain.DeliveryStatusSent {
		t.Errorf("expected notif-ok to have status sent despite earlier failure, got %s", n2.DeliveryStatus)
	}
}

func TestWorker_Lifecycle_InitialPassAndShutdown(t *testing.T) {
	worker, directRepo, _, tplRepo, sender := setupWorkerTest()

	tpl := &domain.EmailTemplate{
		ID:       "tpl-lifecycle",
		Subject:  "Test",
		HTMLBody: "<p>Test</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	n := &domain.DirectNotification{
		ID:               "notif-life",
		TemplateID:       tpl.ID,
		RecipientEmail:   "life@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
	}
	directRepo.notifications[n.ID] = n

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()

	// Wait briefly for initial pass
	time.Sleep(20 * time.Millisecond)

	if n.DeliveryStatus != domain.DeliveryStatusSent {
		t.Errorf("expected notif-life to be delivered on initial pass, got %s", n.DeliveryStatus)
	}
	if len(sender.sentMsgs) != 1 {
		t.Errorf("expected 1 sent message, got %d", len(sender.sentMsgs))
	}

	// Cancel context and verify clean termination
	cancel()
	select {
	case <-done:
		// Clean exit
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker did not exit cleanly upon context cancellation")
	}
}

func TestWorker_ConcurrentInstances_NoDuplicateDelivery(t *testing.T) {
	log, _ := logger.NewLogger("info")
	directRepo := &workerMockRepo{notifications: make(map[string]*domain.DirectNotification)}
	attemptRepo := &workerMockAttemptRepo{attempts: make(map[string]*domain.DeliveryAttempt)}
	tplRepo := &workerMockTplRepo{templates: make(map[string]*domain.EmailTemplate)}
	sender := &workerMockSender{failIDs: make(map[string]bool)}

	deliveryService := NewDeliveryService(directRepo, attemptRepo, tplRepo, sender)

	worker1 := NewWorker(directRepo, deliveryService, 20*time.Millisecond, 5*time.Minute, 5, log)
	worker2 := NewWorker(directRepo, deliveryService, 20*time.Millisecond, 5*time.Minute, 5, log)

	tpl := &domain.EmailTemplate{
		ID:       "tpl-concurrent",
		Subject:  "Subject",
		HTMLBody: "<p>Body</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	// Seed 10 pending notifications
	for i := 1; i <= 10; i++ {
		id := "notif-" + string(rune('A'+i-1))
		directRepo.notifications[id] = &domain.DirectNotification{
			ID:               id,
			TemplateID:       tpl.ID,
			RecipientEmail:   id + "@example.com",
			NotificationType: domain.NotificationTypeDirect,
			DeliveryStatus:   domain.DeliveryStatusPending,
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		worker1.ProcessPending(context.Background())
	}()
	go func() {
		defer wg.Done()
		worker2.ProcessPending(context.Background())
	}()

	wg.Wait()

	if len(sender.sentMsgs) != 10 {
		t.Fatalf("expected exactly 10 messages sent with 0 duplicates, got %d", len(sender.sentMsgs))
	}
}

func TestWorker_StaleRecovery_Delivered(t *testing.T) {
	worker, directRepo, attemptRepo, tplRepo, sender := setupWorkerTest()

	tpl := &domain.EmailTemplate{
		ID:       "tpl-stale",
		Subject:  "Stale Recovered",
		HTMLBody: "<p>Stale Recovered</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	// Stuck in sending with last_attempt_at 10 minutes ago (stale timeout is 5m)
	tenMinsAgo := time.Now().UTC().Add(-10 * time.Minute)
	notifStale := &domain.DirectNotification{
		ID:               "notif-stale-1",
		TemplateID:       tpl.ID,
		RecipientEmail:   "stale@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusSending,
		AttemptsCount:    1,
		LastAttemptAt:    &tenMinsAgo,
	}
	directRepo.notifications[notifStale.ID] = notifStale

	attempt := &domain.DeliveryAttempt{
		ID:            "attempt-stale-1",
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      notifStale.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
		AttemptedAt:   tenMinsAgo,
	}
	attemptRepo.attempts[attempt.ID] = attempt

	// Execute single worker cycle
	worker.ProcessPending(context.Background())

	// Stale notification should have been recovered -> claimed -> delivered to sent
	if notifStale.DeliveryStatus != domain.DeliveryStatusSent {
		t.Errorf("expected recovered stale notification to be sent, got %s", notifStale.DeliveryStatus)
	}
	if len(sender.sentMsgs) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sender.sentMsgs))
	}
}

func TestWorker_StaleRecovery_FreshNotRecovered(t *testing.T) {
	worker, directRepo, _, tplRepo, sender := setupWorkerTest()

	tpl := &domain.EmailTemplate{
		ID:       "tpl-fresh",
		Subject:  "Fresh In Flight",
		HTMLBody: "<p>Fresh In Flight</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	// In sending with last_attempt_at 10 seconds ago (fresh, under 5m)
	tenSecsAgo := time.Now().UTC().Add(-10 * time.Second)
	notifFresh := &domain.DirectNotification{
		ID:               "notif-fresh-1",
		TemplateID:       tpl.ID,
		RecipientEmail:   "fresh@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusSending,
		AttemptsCount:    1,
		LastAttemptAt:    &tenSecsAgo,
	}
	directRepo.notifications[notifFresh.ID] = notifFresh

	worker.ProcessPending(context.Background())

	// Fresh in-flight notification must remain in 'sending' state untouched
	if notifFresh.DeliveryStatus != domain.DeliveryStatusSending {
		t.Errorf("expected fresh notification to remain in sending, got %s", notifFresh.DeliveryStatus)
	}
	if len(sender.sentMsgs) != 0 {
		t.Errorf("expected 0 sent messages for fresh in-flight notification, got %d", len(sender.sentMsgs))
	}
}

func TestWorker_ExhaustedNotification_NeverClaimed(t *testing.T) {
	worker, directRepo, _, tplRepo, sender := setupWorkerTest()

	tpl := &domain.EmailTemplate{
		ID:       "tpl-exhausted",
		Subject:  "Exhausted",
		HTMLBody: "<p>Exhausted</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	notif := &domain.DirectNotification{
		ID:               "notif-exhausted-1",
		TemplateID:       tpl.ID,
		RecipientEmail:   "exhausted@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
		AttemptsCount:    5, // maxAttempts is 5
	}
	directRepo.notifications[notif.ID] = notif

	worker.ProcessPending(context.Background())

	// Must remain in pending and never be claimed/sent
	if notif.DeliveryStatus != domain.DeliveryStatusPending {
		t.Errorf("expected exhausted notification to remain pending (unclaimed), got %s", notif.DeliveryStatus)
	}
	if notif.AttemptsCount != 5 {
		t.Errorf("expected attempts_count to stay 5, got %d", notif.AttemptsCount)
	}
	if len(sender.sentMsgs) != 0 {
		t.Fatalf("expected 0 sent messages, got %d", len(sender.sentMsgs))
	}
}

func TestWorker_RepeatedFailures_EventuallyStopAtMaxAttempts(t *testing.T) {
	log, _ := logger.NewLogger("info")
	directRepo := &workerMockRepo{notifications: make(map[string]*domain.DirectNotification)}
	attemptRepo := &workerMockAttemptRepo{attempts: make(map[string]*domain.DeliveryAttempt)}
	tplRepo := &workerMockTplRepo{templates: make(map[string]*domain.EmailTemplate)}
	sender := &workerMockSender{failIDs: map[string]bool{"failing@example.com": true}}

	maxAttempts := 3
	deliveryService := NewDeliveryServiceWithMaxAttempts(directRepo, attemptRepo, tplRepo, sender, maxAttempts)
	worker := NewWorker(directRepo, deliveryService, 10*time.Millisecond, 5*time.Minute, maxAttempts, log)

	tpl := &domain.EmailTemplate{
		ID:       "tpl-fail",
		Subject:  "Fail",
		HTMLBody: "<p>Fail</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	notif := &domain.DirectNotification{
		ID:               "notif-repeat-fail",
		TemplateID:       tpl.ID,
		RecipientEmail:   "failing@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
		AttemptsCount:    0,
	}
	directRepo.notifications[notif.ID] = notif

	// Cycle 1: attempt 1 fails -> notification returned to pending
	worker.ProcessPending(context.Background())
	if notif.DeliveryStatus != domain.DeliveryStatusPending {
		t.Fatalf("expected status pending after attempt 1, got %s", notif.DeliveryStatus)
	}
	if notif.AttemptsCount != 1 {
		t.Fatalf("expected attempts_count 1, got %d", notif.AttemptsCount)
	}

	// Cycle 2: attempt 2 fails -> notification returned to pending
	worker.ProcessPending(context.Background())
	if notif.DeliveryStatus != domain.DeliveryStatusPending {
		t.Fatalf("expected status pending after attempt 2, got %s", notif.DeliveryStatus)
	}
	if notif.AttemptsCount != 2 {
		t.Fatalf("expected attempts_count 2, got %d", notif.AttemptsCount)
	}

	// Cycle 3: attempt 3 fails -> maxAttempts (3) reached -> permanently failed!
	worker.ProcessPending(context.Background())
	if notif.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Fatalf("expected status failed after attempt 3 (max reached), got %s", notif.DeliveryStatus)
	}
	if notif.AttemptsCount != 3 {
		t.Fatalf("expected attempts_count 3, got %d", notif.AttemptsCount)
	}

	// Cycle 4: subsequent cycle does nothing
	worker.ProcessPending(context.Background())
	if notif.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Fatalf("expected status failed to persist, got %s", notif.DeliveryStatus)
	}
	if notif.AttemptsCount != 3 {
		t.Fatalf("expected attempts_count to stay 3, got %d", notif.AttemptsCount)
	}
}

func TestWorker_StaleRecovery_AtLimit_MarksNotificationFailed(t *testing.T) {
	log, _ := logger.NewLogger("info")
	directRepo := &workerMockRepo{notifications: make(map[string]*domain.DirectNotification)}
	attemptRepo := &workerMockAttemptRepo{attempts: make(map[string]*domain.DeliveryAttempt)}
	tplRepo := &workerMockTplRepo{templates: make(map[string]*domain.EmailTemplate)}
	sender := &workerMockSender{failIDs: make(map[string]bool)}

	maxAttempts := 3
	deliveryService := NewDeliveryServiceWithMaxAttempts(directRepo, attemptRepo, tplRepo, sender, maxAttempts)
	worker := NewWorker(directRepo, deliveryService, 10*time.Millisecond, 5*time.Minute, maxAttempts, log)

	tpl := &domain.EmailTemplate{
		ID:       "tpl-stale-limit",
		Subject:  "Stale Limit",
		HTMLBody: "<p>Stale Limit</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	// In sending state with attempts_count = 3 (at limit) and last_attempt_at 10 minutes ago
	tenMinsAgo := time.Now().UTC().Add(-10 * time.Minute)
	notif := &domain.DirectNotification{
		ID:               "notif-stale-limit-1",
		TemplateID:       tpl.ID,
		RecipientEmail:   "stale-limit@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusSending,
		AttemptsCount:    3,
		LastAttemptAt:    &tenMinsAgo,
	}
	directRepo.notifications[notif.ID] = notif

	worker.ProcessPending(context.Background())

	// Stale recovery at limit must mark notification permanently as failed
	if notif.DeliveryStatus != domain.DeliveryStatusFailed {
		t.Errorf("expected stale notification at limit to become failed, got %s", notif.DeliveryStatus)
	}
	if len(sender.sentMsgs) != 0 {
		t.Errorf("expected 0 sent messages, got %d", len(sender.sentMsgs))
	}
}
