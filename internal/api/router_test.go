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
	"github.com/gmhelper/notify-api/internal/app/template"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

const (
	routerTestSecret   = "cm91dGVyLXRlc3Qtc2VjcmV0LWtleS0zMi1jaGFycyE="
	routerTestIssuer   = "gmhelper-api"
	routerTestAudience = "gmhelper-notify-api"
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

func (m *routerMockDirectRepo) ClaimPending(ctx context.Context, limit int, maxAttempts int) ([]*domain.DirectNotification, error) {
	return []*domain.DirectNotification{
		{ID: "pending-notif-1", DeliveryStatus: domain.DeliveryStatusSending, AttemptsCount: 1},
	}, nil
}

func (m *routerMockDirectRepo) RecoverStaleSending(ctx context.Context, olderThan time.Duration, maxAttempts int) (int64, error) {
	return 0, nil
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
	router := NewRouter(healthHandler, nil, nil, nil)

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
	router := NewRouter(healthHandler, nil, nil, nil)

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

func TestRouter_DirectNotificationsRouting_AuthAndPrecedence(t *testing.T) {
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

	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.Authenticate(verifier, log)

	router := NewRouter(nil, nil, directHandler, authMw)

	validToken, err := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "user-admin", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 1. Unauthenticated request to /api/v1/notifications/direct/pending -> 401 Unauthorized
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/pending", nil)
	recUnauth := httptest.NewRecorder()
	router.ServeHTTP(recUnauth, reqUnauth)

	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected HTTP 401 Unauthorized for unauthenticated request, got %d", recUnauth.Code)
	}

	// 2. Authenticated request to /api/v1/notifications/direct/pending -> 200 OK (ListPending)
	reqPending := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/pending", nil)
	reqPending.Header.Set("Authorization", "Bearer "+validToken)
	recPending := httptest.NewRecorder()
	router.ServeHTTP(recPending, reqPending)

	if recPending.Code != http.StatusOK {
		t.Fatalf("expected status 200 on authenticated /pending, got %d", recPending.Code)
	}

	var list []handlers.DirectNotificationResponse
	if err := json.Unmarshal(recPending.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode JSON list from /pending: %v (body: %s)", err, recPending.Body.String())
	}
	if len(list) != 1 || list[0].ID != "pending-notif-1" {
		t.Errorf("expected ListPending response, got: %s", recPending.Body.String())
	}

	// 3. Authenticated request to /api/v1/notifications/direct/actual-id-1 -> 200 OK (GetByID)
	reqByID := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/actual-id-1", nil)
	reqByID.Header.Set("Authorization", "Bearer "+validToken)
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

func TestRouter_AdministrativeRoutes_SecurityMatrix(t *testing.T) {
	log, _ := logger.NewLogger("info")
	directRepo := &routerMockDirectRepo{
		notifications: map[string]*domain.DirectNotification{
			"test-notif-1": {
				ID:             "test-notif-1",
				RecipientEmail: "recipient@example.com",
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

	templateService := template.NewService(tplRepo)
	templateHandler := handlers.NewTemplateHandler(templateService, log)

	pinger := &dummyPinger{err: nil}
	readiness := health.NewReadinessService(pinger)
	healthHandler := handlers.NewHealthHandler(readiness, log)

	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.AdminAuth(verifier, log)

	router := NewRouter(healthHandler, templateHandler, directHandler, authMw)

	adminToken, _ := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "u-admin", "admin", 15*time.Minute)
	ownerToken, _ := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "u-owner", "owner", 15*time.Minute)
	serviceToken, _ := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "u-svc", "service", 15*time.Minute)
	userToken, _ := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "u-regular", "user", 15*time.Minute)
	expiredToken, _ := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "u-admin", "admin", -5*time.Minute)
	badSigToken, _ := auth.GenerateToken("wrong-key-with-32-characters!!!!", routerTestIssuer, routerTestAudience, "u-admin", "admin", 15*time.Minute)

	type endpointTest struct {
		method string
		path   string
	}

	endpoints := []endpointTest{
		{method: http.MethodGet, path: "/api/v1/templates"},
		{method: http.MethodPost, path: "/api/v1/templates"},
		{method: http.MethodGet, path: "/api/v1/templates/tpl-123"},
		{method: http.MethodPut, path: "/api/v1/templates/tpl-123"},
		{method: http.MethodDelete, path: "/api/v1/templates/tpl-123"},
		{method: http.MethodPost, path: "/api/v1/notifications/direct"},
		{method: http.MethodGet, path: "/api/v1/notifications/direct/pending"},
		{method: http.MethodGet, path: "/api/v1/notifications/direct/test-notif-1"},
		{method: http.MethodPost, path: "/api/v1/notifications/direct/test-notif-1/deliver"},
	}

	for _, ep := range endpoints {
		// 1. Unauthenticated -> 401
		t.Run(ep.method+" "+ep.path+" [Unauthenticated -> 401]", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 Unauthorized, got %d (body: %s)", rec.Code, rec.Body.String())
			}
		})

		// 2. Expired Token -> 401
		t.Run(ep.method+" "+ep.path+" [Expired JWT -> 401]", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+expiredToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 Unauthorized for expired token, got %d", rec.Code)
			}
		})

		// 3. Bad Signature -> 401
		t.Run(ep.method+" "+ep.path+" [Bad Signature -> 401]", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+badSigToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 Unauthorized for bad signature, got %d", rec.Code)
			}
		})

		// 4. Non-Admin Role (user) -> 403 Forbidden
		t.Run(ep.method+" "+ep.path+" [Role User -> 403]", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+userToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden for role 'user', got %d (body: %s)", rec.Code, rec.Body.String())
			}
		})

		// 5. Admin Role -> Authorized (passes auth boundary)
		t.Run(ep.method+" "+ep.path+" [Role Admin -> Allowed]", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("expected request to pass auth boundary for admin role, got %d", rec.Code)
			}
		})

		// 6. Owner Role -> Authorized (passes auth boundary)
		t.Run(ep.method+" "+ep.path+" [Role Owner -> Allowed]", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+ownerToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("expected request to pass auth boundary for owner role, got %d", rec.Code)
			}
		})

		// 7. Service Role -> Authorized (passes auth boundary)
		t.Run(ep.method+" "+ep.path+" [Role Service -> Allowed]", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+serviceToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("expected request to pass auth boundary for service role, got %d", rec.Code)
			}
		})
	}
}
