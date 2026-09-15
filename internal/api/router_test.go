package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/api/handlers"
	"github.com/gmhelper/notify-api/internal/app/campaign"
	"github.com/gmhelper/notify-api/internal/app/dashboard"
	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/app/health"
	"github.com/gmhelper/notify-api/internal/app/template"
	"github.com/gmhelper/notify-api/internal/app/user"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/userclient"
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

type routerMockCampaignRepo struct {
	campaigns map[string]*domain.NotificationCampaign
}

func (m *routerMockCampaignRepo) GetByID(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}
func (m *routerMockCampaignRepo) Create(ctx context.Context, c *domain.NotificationCampaign) error {
	m.campaigns[c.ID] = c
	return nil
}
func (m *routerMockCampaignRepo) Update(ctx context.Context, c *domain.NotificationCampaign) error {
	if _, ok := m.campaigns[c.ID]; ok {
		m.campaigns[c.ID] = c
		return nil
	}
	return domain.ErrNotFound
}
func (m *routerMockCampaignRepo) Delete(ctx context.Context, id string) error {
	if _, ok := m.campaigns[id]; ok {
		delete(m.campaigns, id)
		return nil
	}
	return domain.ErrNotFound
}
func (m *routerMockCampaignRepo) UpdateStatus(ctx context.Context, id string, status domain.CampaignStatus, startedAt, completedAt *time.Time) error {
	return nil
}
func (m *routerMockCampaignRepo) ListByStatus(ctx context.Context, status domain.CampaignStatus) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}
func (m *routerMockCampaignRepo) ListScheduled(ctx context.Context, after time.Time) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}
func (m *routerMockCampaignRepo) ListDue(ctx context.Context, dueBefore time.Time, limit int) ([]*domain.NotificationCampaign, error) {
	return nil, nil
}
func (m *routerMockCampaignRepo) Claim(ctx context.Context, id string) (*domain.NotificationCampaign, error) {
	if c, ok := m.campaigns[id]; ok {
		if c.Status == domain.CampaignStatusScheduled {
			c.Status = domain.CampaignStatusRunning
			return c, nil
		}
		return nil, domain.ErrNotFound
	}
	return nil, domain.ErrNotFound
}
func (m *routerMockCampaignRepo) List(ctx context.Context) ([]*domain.NotificationCampaign, error) {
	return []*domain.NotificationCampaign{
		{ID: "camp-1", Name: "Campaign 1", Status: domain.CampaignStatusDraft},
	}, nil
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
	router := NewRouter(healthHandler, nil, nil, nil, nil, nil, nil)

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
	router := NewRouter(healthHandler, nil, nil, nil, nil, nil, nil)

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

	directService := direct.NewService(tplRepo, directRepo, nil)
	deliveryService := direct.NewDeliveryService(directRepo, attemptRepo, tplRepo, sender)
	directHandler := handlers.NewDirectNotificationHandler(directService, deliveryService, log)

	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.Authenticate(verifier, log)

	router := NewRouter(nil, nil, nil, directHandler, nil, nil, authMw)

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

	directService := direct.NewService(tplRepo, directRepo, nil)
	deliveryService := direct.NewDeliveryService(directRepo, attemptRepo, tplRepo, sender)
	directHandler := handlers.NewDirectNotificationHandler(directService, deliveryService, log)

	templateService := template.NewService(tplRepo)
	templateHandler := handlers.NewTemplateHandler(templateService, log)

	campaignRepo := &routerMockCampaignRepo{campaigns: make(map[string]*domain.NotificationCampaign)}
	campaignService := campaign.NewService(campaignRepo)
	campaignHandler := handlers.NewCampaignHandler(campaignService, log)

	pinger := &dummyPinger{err: nil}
	readiness := health.NewReadinessService(pinger)
	healthHandler := handlers.NewHealthHandler(readiness, log)

	mockResolver := &routerMockUserResolver{}
	userService, _ := user.NewService(mockResolver)
	userHandler := handlers.NewUserHandler(userService, log)

	dashboardRepo := &routerMockDashboardRepo{}
	dashboardService := dashboard.NewService(dashboardRepo)
	dashboardHandler := handlers.NewDashboardHandler(dashboardService, log)

	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.AdminAuth(verifier, log)

	router := NewRouter(healthHandler, templateHandler, campaignHandler, directHandler, userHandler, dashboardHandler, authMw)

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
		{method: http.MethodPost, path: "/api/v1/templates/tpl-123/preview"},
		{method: http.MethodGet, path: "/api/v1/campaigns"},
		{method: http.MethodPost, path: "/api/v1/campaigns"},
		{method: http.MethodGet, path: "/api/v1/campaigns/camp-123"},
		{method: http.MethodPut, path: "/api/v1/campaigns/camp-123"},
		{method: http.MethodPost, path: "/api/v1/notifications/direct"},
		{method: http.MethodGet, path: "/api/v1/notifications/direct/pending"},
		{method: http.MethodGet, path: "/api/v1/notifications/direct/test-notif-1"},
		{method: http.MethodPost, path: "/api/v1/notifications/direct/test-notif-1/deliver"},
		{method: http.MethodGet, path: "/api/v1/users/search"},
		{method: http.MethodGet, path: "/api/v1/dashboard/stats"},
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

type routerMockDashboardRepo struct {
	stats *domain.DashboardStats
	err   error
}

func (m *routerMockDashboardRepo) GetDashboardStats(ctx context.Context, recentLimit int) (*domain.DashboardStats, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.stats != nil {
		return m.stats, nil
	}
	return &domain.DashboardStats{
		Campaigns:       domain.DashboardCampaignStats{},
		Templates:       domain.DashboardTemplateStats{},
		Deliveries:      domain.DashboardDeliveryStats{},
		RecentCampaigns: []*domain.RecentCampaignItem{},
	}, nil
}

type routerMockUserResolver struct {
	searchUsersFunc func(ctx context.Context, query string, limit int) ([]userclient.User, error)
}

func (m *routerMockUserResolver) GetUserByID(ctx context.Context, id string) (*userclient.User, error) {
	return nil, nil
}

func (m *routerMockUserResolver) SearchUsers(ctx context.Context, query string, limit int) ([]userclient.User, error) {
	if m.searchUsersFunc != nil {
		return m.searchUsersFunc(ctx, query, limit)
	}
	return nil, nil
}

func TestRouter_UserSearchRouting(t *testing.T) {
	log, _ := logger.NewLogger("info")
	mockResolver := &routerMockUserResolver{
		searchUsersFunc: func(ctx context.Context, query string, limit int) ([]userclient.User, error) {
			if query == "alice" {
				return []userclient.User{
					{
						ID:       "user-alice-id",
						Username: "alice",
						Email:    "alice@example.com",
						Role:     "User",
						Language: "EN",
						IsActive: true,
					},
				}, nil
			}
			return []userclient.User{}, nil
		},
	}
	userService, _ := user.NewService(mockResolver)
	userHandler := handlers.NewUserHandler(userService, log)

	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.AdminAuth(verifier, log)

	router := NewRouter(nil, nil, nil, nil, userHandler, nil, authMw)

	adminToken, _ := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "u-admin", "admin", 15*time.Minute)

	// 1. Search with matching query
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/search?q=alice", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var results []handlers.UserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(results) != 1 || results[0].Username != "alice" {
		t.Errorf("unexpected results: %+v", results)
	}

	// 2. Search without query -> 400 Bad Request
	reqEmpty := httptest.NewRequest(http.MethodGet, "/api/v1/users/search", nil)
	reqEmpty.Header.Set("Authorization", "Bearer "+adminToken)
	recEmpty := httptest.NewRecorder()
	router.ServeHTTP(recEmpty, reqEmpty)

	if recEmpty.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for missing query, got %d", recEmpty.Code)
	}
}

func TestRouter_CampaignEndpoints(t *testing.T) {
	log, _ := logger.NewLogger("error")
	campaignRepo := &routerMockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"camp-edit-1": {
				ID:           "camp-edit-1",
				Name:         "Initial Campaign",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			},
		},
	}
	campaignService := campaign.NewService(campaignRepo)
	campaignHandler := handlers.NewCampaignHandler(campaignService, log)

	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.AdminAuth(verifier, log)

	router := NewRouter(nil, nil, campaignHandler, nil, nil, nil, authMw)

	adminToken, _ := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "u-admin", "admin", 15*time.Minute)

	// 1. GET /api/v1/campaigns/camp-edit-1
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-edit-1", nil)
	reqGet.Header.Set("Authorization", "Bearer "+adminToken)
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET campaign, got %d (body: %s)", recGet.Code, recGet.Body.String())
	}

	// 2. PUT /api/v1/campaigns/camp-edit-1
	updatePayload := map[string]string{
		"name":         "Updated Campaign Name",
		"templateId":   "tpl-2",
		"campaignType": "broadcast",
	}
	bodyBytes, _ := json.Marshal(updatePayload)
	reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/campaigns/camp-edit-1", bytes.NewReader(bodyBytes))
	reqPut.Header.Set("Authorization", "Bearer "+adminToken)
	reqPut.Header.Set("Content-Type", "application/json")
	recPut := httptest.NewRecorder()
	router.ServeHTTP(recPut, reqPut)

	if recPut.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for PUT campaign, got %d (body: %s)", recPut.Code, recPut.Body.String())
	}

	var res handlers.CampaignResponse
	if err := json.Unmarshal(recPut.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if res.Name != "Updated Campaign Name" || res.TemplateID != "tpl-2" {
		t.Errorf("unexpected response content: %+v", res)
	}

	// Verify repo was updated
	updatedCamp, _ := campaignRepo.GetByID(context.Background(), "camp-edit-1")
	if updatedCamp.Name != "Updated Campaign Name" {
		t.Errorf("expected repo campaign name 'Updated Campaign Name', got '%s'", updatedCamp.Name)
	}
}

func TestRouter_CampaignDelete(t *testing.T) {
	log, _ := logger.NewLogger("info")
	campaignRepo := &routerMockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"camp-del-1": {
				ID:           "camp-del-1",
				Name:         "Campaign To Delete",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
			},
		},
	}
	campaignService := campaign.NewService(campaignRepo)
	campaignHandler := handlers.NewCampaignHandler(campaignService, log)

	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.Authenticate(verifier, log)

	router := NewRouter(nil, nil, campaignHandler, nil, nil, nil, authMw)

	adminToken, err := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "user-admin", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 1. Unauthenticated DELETE -> 401 Unauthorized
	reqUnauth := httptest.NewRequest(http.MethodDelete, "/api/v1/campaigns/camp-del-1", nil)
	recUnauth := httptest.NewRecorder()
	router.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", recUnauth.Code)
	}

	// 2. Authenticated DELETE /api/v1/campaigns/camp-del-1 -> 204 No Content
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/campaigns/camp-del-1", nil)
	reqDel.Header.Set("Authorization", "Bearer "+adminToken)
	recDel := httptest.NewRecorder()
	router.ServeHTTP(recDel, reqDel)

	if recDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for DELETE campaign, got %d (body: %s)", recDel.Code, recDel.Body.String())
	}

	// 3. Repeated DELETE -> 404 Not Found
	reqRepeat := httptest.NewRequest(http.MethodDelete, "/api/v1/campaigns/camp-del-1", nil)
	reqRepeat.Header.Set("Authorization", "Bearer "+adminToken)
	recRepeat := httptest.NewRecorder()
	router.ServeHTTP(recRepeat, reqRepeat)

	if recRepeat.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found on repeat DELETE, got %d", recRepeat.Code)
	}
}

func TestRouter_CampaignScheduleAndCancel(t *testing.T) {
	log, _ := logger.NewLogger("info")
	now := time.Now().UTC()
	futureTime := now.Add(24 * time.Hour)

	campaignRepo := &routerMockCampaignRepo{
		campaigns: map[string]*domain.NotificationCampaign{
			"camp-sched-1": {
				ID:           "camp-sched-1",
				Name:         "Campaign To Schedule",
				TemplateID:   "tpl-1",
				CampaignType: "broadcast",
				Status:       domain.CampaignStatusDraft,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}
	campaignService := campaign.NewService(campaignRepo)
	campaignHandler := handlers.NewCampaignHandler(campaignService, log)

	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.AdminAuth(verifier, log)

	router := NewRouter(nil, nil, campaignHandler, nil, nil, nil, authMw)

	adminToken, err := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "user-admin", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 1. Unauthenticated schedule -> 401 Unauthorized
	schedulePayload, _ := json.Marshal(map[string]interface{}{
		"scheduledAt": futureTime,
	})
	reqUnauthSched := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-sched-1/schedule", bytes.NewReader(schedulePayload))
	recUnauthSched := httptest.NewRecorder()
	router.ServeHTTP(recUnauthSched, reqUnauthSched)
	if recUnauthSched.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauth schedule, got %d", recUnauthSched.Code)
	}

	// 2. Unauthenticated cancel -> 401 Unauthorized
	reqUnauthCancel := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-sched-1/cancel", nil)
	recUnauthCancel := httptest.NewRecorder()
	router.ServeHTTP(recUnauthCancel, reqUnauthCancel)
	if recUnauthCancel.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauth cancel, got %d", recUnauthCancel.Code)
	}

	// 3. Authenticated Schedule -> 200 OK
	reqSched := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-sched-1/schedule", bytes.NewReader(schedulePayload))
	reqSched.Header.Set("Authorization", "Bearer "+adminToken)
	reqSched.Header.Set("Content-Type", "application/json")
	recSched := httptest.NewRecorder()
	router.ServeHTTP(recSched, reqSched)

	if recSched.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for schedule, got %d: %s", recSched.Code, recSched.Body.String())
	}

	var resSched handlers.CampaignResponse
	if err := json.Unmarshal(recSched.Body.Bytes(), &resSched); err != nil {
		t.Fatalf("failed to unmarshal schedule response: %v", err)
	}
	if resSched.Status != "scheduled" || resSched.ScheduledAt == nil {
		t.Fatalf("expected scheduled status and non-nil scheduledAt, got %+v", resSched)
	}

	// 4. Authenticated Cancel -> 200 OK
	reqCancel := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-sched-1/cancel", nil)
	reqCancel.Header.Set("Authorization", "Bearer "+adminToken)
	recCancel := httptest.NewRecorder()
	router.ServeHTTP(recCancel, reqCancel)

	if recCancel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for cancel, got %d: %s", recCancel.Code, recCancel.Body.String())
	}

	var resCancel handlers.CampaignResponse
	if err := json.Unmarshal(recCancel.Body.Bytes(), &resCancel); err != nil {
		t.Fatalf("failed to unmarshal cancel response: %v", err)
	}
	if resCancel.Status != "cancelled" || resCancel.ScheduledAt == nil {
		t.Fatalf("expected cancelled status with preserved scheduledAt, got %+v", resCancel)
	}
}

func TestRouter_DashboardStats(t *testing.T) {
	log, _ := logger.NewLogger("info")
	dashboardRepo := &routerMockDashboardRepo{
		stats: &domain.DashboardStats{
			Campaigns: domain.DashboardCampaignStats{
				Total:     10,
				Draft:     2,
				Scheduled: 1,
				Running:   1,
				Completed: 5,
				Cancelled: 1,
			},
			Templates: domain.DashboardTemplateStats{
				Total:    8,
				Active:   6,
				Archived: 2,
			},
			Deliveries: domain.DashboardDeliveryStats{
				TotalMessages: 500,
				TotalSent:     480,
				TotalFailed:   15,
				TotalPending:  5,
			},
			RecentCampaigns: []*domain.RecentCampaignItem{
				{
					ID:           "c1",
					Name:         "Spring Promo",
					CampaignType: "broadcast",
					Status:       domain.CampaignStatusCompleted,
					CreatedAt:    time.Now().UTC(),
				},
			},
		},
	}
	dashboardService := dashboard.NewService(dashboardRepo)
	dashboardHandler := handlers.NewDashboardHandler(dashboardService, log)

	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.AdminAuth(verifier, log)

	router := NewRouter(nil, nil, nil, nil, nil, dashboardHandler, authMw)

	adminToken, err := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "user-admin", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 1. Unauthenticated -> 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	recUnauth := httptest.NewRecorder()
	router.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauth request, got %d", recUnauth.Code)
	}

	// 2. Authenticated -> 200 OK
	reqAuth := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats?limit=5", nil)
	reqAuth.Header.Set("Authorization", "Bearer "+adminToken)
	recAuth := httptest.NewRecorder()
	router.ServeHTTP(recAuth, reqAuth)

	if recAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", recAuth.Code, recAuth.Body.String())
	}

	var stats domain.DashboardStats
	if err := json.Unmarshal(recAuth.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}
	if stats.Campaigns.Total != 10 || stats.Templates.Total != 8 || stats.Deliveries.TotalSent != 480 {
		t.Errorf("unexpected dashboard stats in response: %+v", stats)
	}
	if len(stats.RecentCampaigns) != 1 || stats.RecentCampaigns[0].Name != "Spring Promo" {
		t.Errorf("unexpected recent campaigns in response: %+v", stats.RecentCampaigns)
	}
}
