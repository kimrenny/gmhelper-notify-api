package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/api/handlers"
	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/app/health"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type dummyPinger struct {
	err error
}

func (d *dummyPinger) Ping(ctx context.Context) error {
	return d.err
}

type routerMockDirectRepo struct {
	notifications map[string]*domain.DirectNotification
}

func (m *routerMockDirectRepo) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	if n, ok := m.notifications[id]; ok {
		return n, nil
	}
	return nil, domain.ErrNotFound
}

func (m *routerMockDirectRepo) Create(ctx context.Context, n *domain.DirectNotification) error {
	return nil
}

func (m *routerMockDirectRepo) CreateWithInitialAttempt(ctx context.Context, n *domain.DirectNotification, a *domain.DeliveryAttempt) error {
	return nil
}

func (m *routerMockDirectRepo) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	return []*domain.DirectNotification{
		{ID: "pending-notif-1", DeliveryStatus: domain.DeliveryStatusPending},
	}, nil
}

func (m *routerMockDirectRepo) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errMsg string) error {
	return nil
}

type routerMockAttemptRepo struct{}

func (m *routerMockAttemptRepo) GetByID(ctx context.Context, id string) (*domain.DeliveryAttempt, error) {
	return nil, domain.ErrNotFound
}
func (m *routerMockAttemptRepo) Create(ctx context.Context, a *domain.DeliveryAttempt) error {
	return nil
}
func (m *routerMockAttemptRepo) Update(ctx context.Context, a *domain.DeliveryAttempt) error {
	return nil
}
func (m *routerMockAttemptRepo) ListByTarget(ctx context.Context, t domain.DeliveryTargetType, id string) ([]*domain.DeliveryAttempt, error) {
	return nil, nil
}

type routerMockTplRepo struct{}

func (m *routerMockTplRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	return nil, domain.ErrNotFound
}
func (m *routerMockTplRepo) GetByKey(ctx context.Context, key string) (*domain.EmailTemplate, error) {
	return nil, domain.ErrNotFound
}
func (m *routerMockTplRepo) Create(ctx context.Context, t *domain.EmailTemplate) error {
	return nil
}
func (m *routerMockTplRepo) Update(ctx context.Context, t *domain.EmailTemplate) error {
	return nil
}
func (m *routerMockTplRepo) Delete(ctx context.Context, id string) error {
	return nil
}
func (m *routerMockTplRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

type routerMockSender struct{}

func (m *routerMockSender) Send(ctx context.Context, msg *email.Message) error {
	return nil
}

func TestRouter_HealthAndReady(t *testing.T) {
	log, _ := logger.NewLogger("info")
	pinger := &dummyPinger{err: nil}
	readiness := health.NewReadinessService(pinger)
	healthHandler := handlers.NewHealthHandler(readiness, log)
	router := NewRouter(healthHandler, nil, nil)

	// 1. GET /health
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	router.ServeHTTP(recHealth, reqHealth)

	if recHealth.Code != http.StatusOK {
		t.Errorf("expected status 200 on /health, got %d", recHealth.Code)
	}

	// 2. GET /ready (healthy)
	reqReady := httptest.NewRequest(http.MethodGet, "/ready", nil)
	recReady := httptest.NewRecorder()
	router.ServeHTTP(recReady, reqReady)

	if recReady.Code != http.StatusOK {
		t.Errorf("expected status 200 on /ready, got %d", recReady.Code)
	}

	// 3. GET /ready (unhealthy)
	pinger.err = errors.New("db down")
	recReadyDown := httptest.NewRecorder()
	router.ServeHTTP(recReadyDown, reqReady)

	if recReadyDown.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 on /ready when db down, got %d", recReadyDown.Code)
	}
}

func TestRouter_NotFoundJSON(t *testing.T) {
	log, _ := logger.NewLogger("info")
	pinger := &dummyPinger{err: nil}
	readiness := health.NewReadinessService(pinger)
	healthHandler := handlers.NewHealthHandler(readiness, log)
	router := NewRouter(healthHandler, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown-endpoint", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected valid JSON error response, got error: %v (body: %s)", err, rec.Body.String())
	}

	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected error code NOT_FOUND, got %s", errResp.Error.Code)
	}
}

func TestRouter_DirectNotificationsRouting_PendingPrecedence(t *testing.T) {
	log, _ := logger.NewLogger("info")
	directRepo := &routerMockDirectRepo{
		notifications: map[string]*domain.DirectNotification{
			"pending": {
				ID:             "pending",
				RecipientEmail: "pending-as-id@example.com",
				DeliveryStatus: domain.DeliveryStatusPending,
			},
			"actual-id-1": {
				ID:             "actual-id-1",
				RecipientEmail: "actual@example.com",
				DeliveryStatus: domain.DeliveryStatusPending,
			},
		},
	}
	attemptRepo := &routerMockAttemptRepo{}
	tplRepo := &routerMockTplRepo{}
	sender := &routerMockSender{}

	directService := direct.NewService(tplRepo, directRepo)
	deliveryService := direct.NewDeliveryService(directRepo, attemptRepo, tplRepo, sender)
	directHandler := handlers.NewDirectNotificationHandler(directService, deliveryService, log)

	router := NewRouter(nil, nil, directHandler)

	// 1. GET /api/v1/notifications/direct/pending MUST return array of pending notifications (ListPending)
	reqPending := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/pending", nil)
	recPending := httptest.NewRecorder()
	router.ServeHTTP(recPending, reqPending)

	if recPending.Code != http.StatusOK {
		t.Fatalf("expected status 200 on /pending, got %d", recPending.Code)
	}

	var list []handlers.DirectNotificationResponse
	if err := json.Unmarshal(recPending.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode JSON list from /pending: %v (body: %s)", err, recPending.Body.String())
	}
	if len(list) != 1 || list[0].ID != "pending-notif-1" {
		t.Errorf("expected ListPending response, got: %s", recPending.Body.String())
	}

	// 2. GET /api/v1/notifications/direct/actual-id-1 MUST return single notification object
	reqByID := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/actual-id-1", nil)
	recByID := httptest.NewRecorder()
	router.ServeHTTP(recByID, reqByID)

	if recByID.Code != http.StatusOK {
		t.Fatalf("expected status 200 on /notifications/direct/actual-id-1, got %d", recByID.Code)
	}

	var single handlers.DirectNotificationResponse
	if err := json.Unmarshal(recByID.Body.Bytes(), &single); err != nil {
		t.Fatalf("failed to decode single notification: %v", err)
	}
	if single.ID != "actual-id-1" {
		t.Errorf("expected ID actual-id-1, got %s", single.ID)
	}
}
