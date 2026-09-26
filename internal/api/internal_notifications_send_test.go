package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/api/handlers"
	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type sendInternalMockSender struct {
	sentMsgs []*email.Message
}

func (s *sendInternalMockSender) Send(ctx context.Context, msg *email.Message) error {
	s.sentMsgs = append(s.sentMsgs, msg)
	return nil
}

type sendInternalTemplateRepo struct {
	templates map[string]*domain.EmailTemplate
}

func (m *sendInternalTemplateRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	if t, ok := m.templates[id]; ok {
		return t, nil
	}
	return nil, domain.ErrNotFound
}
func (m *sendInternalTemplateRepo) GetByKey(ctx context.Context, key string) (*domain.EmailTemplate, error) {
	for _, t := range m.templates {
		if t.TemplateKey == key {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *sendInternalTemplateRepo) GetByKeyAndLocale(ctx context.Context, key, locale string) (*domain.EmailTemplate, error) {
	for _, t := range m.templates {
		if t.TemplateKey == key && (t.Locale == locale || locale == "") {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *sendInternalTemplateRepo) Create(ctx context.Context, t *domain.EmailTemplate) error {
	return nil
}
func (m *sendInternalTemplateRepo) Update(ctx context.Context, t *domain.EmailTemplate) error {
	return nil
}
func (m *sendInternalTemplateRepo) Delete(ctx context.Context, id string) error { return nil }
func (m *sendInternalTemplateRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

type sendInternalDirectRepo struct {
	notifications map[string]*domain.DirectNotification
}

func (m *sendInternalDirectRepo) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	if n, ok := m.notifications[id]; ok {
		return n, nil
	}
	return nil, domain.ErrNotFound
}
func (m *sendInternalDirectRepo) Create(ctx context.Context, n *domain.DirectNotification) error {
	m.notifications[n.ID] = n
	return nil
}
func (m *sendInternalDirectRepo) CreateWithInitialAttempt(ctx context.Context, n *domain.DirectNotification, a *domain.DeliveryAttempt) error {
	m.notifications[n.ID] = n
	return nil
}
func (m *sendInternalDirectRepo) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	var list []*domain.DirectNotification
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusPending {
			list = append(list, n)
		}
	}
	return list, nil
}
func (m *sendInternalDirectRepo) ClaimPending(ctx context.Context, limit int, maxAttempts int) ([]*domain.DirectNotification, error) {
	return nil, nil
}
func (m *sendInternalDirectRepo) RecoverStaleSending(ctx context.Context, olderThan time.Duration, maxAttempts int) (int64, error) {
	return 0, nil
}
func (m *sendInternalDirectRepo) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error {
	if n, ok := m.notifications[id]; ok {
		n.DeliveryStatus = status
		n.AttemptsCount = attempts
		n.LastAttemptAt = lastAttemptAt
		n.SentAt = sentAt
		n.ErrorMessage = errorMessage
	}
	return nil
}

type sendInternalAttemptRepo struct {
	attempts map[string]*domain.DeliveryAttempt
}

func (m *sendInternalAttemptRepo) GetByID(ctx context.Context, id string) (*domain.DeliveryAttempt, error) {
	if a, ok := m.attempts[id]; ok {
		return a, nil
	}
	return nil, domain.ErrNotFound
}
func (m *sendInternalAttemptRepo) Create(ctx context.Context, a *domain.DeliveryAttempt) error {
	m.attempts[a.ID] = a
	return nil
}
func (m *sendInternalAttemptRepo) Update(ctx context.Context, a *domain.DeliveryAttempt) error {
	m.attempts[a.ID] = a
	return nil
}
func (m *sendInternalAttemptRepo) ListByTarget(ctx context.Context, targetType domain.DeliveryTargetType, targetID string) ([]*domain.DeliveryAttempt, error) {
	return nil, nil
}

type sendInternalActivityRepo struct {
	created []*domain.ActivityLog
}

func (m *sendInternalActivityRepo) Create(ctx context.Context, log *domain.ActivityLog) error {
	m.created = append(m.created, log)
	return nil
}
func (m *sendInternalActivityRepo) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	return m.created, len(m.created), nil
}
func (m *sendInternalActivityRepo) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	return nil, domain.ErrNotFound
}

func TestIntegration_InternalNotificationSendEndpoint(t *testing.T) {
	log, _ := logger.NewLogger("error")
	verifier := auth.MustNewJWTVerifier(routerTestSecret, routerTestIssuer, routerTestAudience)
	authMw := middleware.AdminAuth(verifier, log)

	tplRepo := &sendInternalTemplateRepo{
		templates: map[string]*domain.EmailTemplate{
			"tpl-reg-en": {
				ID:           "tpl-reg-en",
				TemplateKey:  "auth.register_code",
				Name:         "Register Code EN",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Your GMHelper verification code: {{ code }}",
				HTMLBody:     "<p>Code: {{ code }}</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusActive,
				Version:      1,
			},
			"tpl-reg-ru": {
				ID:           "tpl-reg-ru",
				TemplateKey:  "auth.register_code",
				Name:         "Register Code RU",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Ваш проверочный код GMHelper: {{ code }}",
				HTMLBody:     "<p>Код: {{ code }}</p>",
				Locale:       "ru",
				Status:       domain.TemplateStatusActive,
			},
			"tpl-pw-en": {
				ID:           "tpl-pw-en",
				TemplateKey:  "auth.password_recovery",
				Name:         "Password Recovery EN",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Reset Password",
				HTMLBody:     "<p>Click <a href=\"{{ recoveryLink }}\">here</a> to reset</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusActive,
			},
			"tpl-inactive": {
				ID:           "tpl-inactive",
				TemplateKey:  "auth.draft_tpl",
				Name:         "Draft Template",
				TemplateType: domain.TemplateTypeDirect,
				Subject:      "Draft",
				HTMLBody:     "<p>Draft</p>",
				Locale:       "en",
				Status:       domain.TemplateStatusDraft,
			},
		},
	}

	directRepo := &sendInternalDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
	}
	attemptRepo := &sendInternalAttemptRepo{
		attempts: make(map[string]*domain.DeliveryAttempt),
	}
	auditRepo := &sendInternalActivityRepo{}
	auditSvc := audit.NewService(auditRepo)
	sender := &sendInternalMockSender{}

	directSvc := direct.NewService(tplRepo, directRepo, &routerMockUserResolver{}, auditSvc)
	deliverySvc := direct.NewDeliveryService(directRepo, attemptRepo, tplRepo, sender, auditSvc)
	directHandler := handlers.NewDirectNotificationHandler(directSvc, deliverySvc, log)

	router := NewRouter(nil, nil, nil, directHandler, nil, nil, nil, nil, nil, nil, nil, authMw)

	// Auth tokens
	serviceToken, err := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "svc-gmhelper-api", "service", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate service token: %v", err)
	}
	userToken, err := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "user-1", "user", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate user token: %v", err)
	}
	adminToken, err := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "admin-1", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate admin token: %v", err)
	}
	expiredToken, err := auth.GenerateToken(routerTestSecret, routerTestIssuer, routerTestAudience, "svc-gmhelper-api", "service", -15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}
	invalidSigToken, err := auth.GenerateToken("d3Jvbmctc2VjcmV0LWtleS0zMi1jaGFycyE=", routerTestIssuer, routerTestAudience, "svc-gmhelper-api", "service", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate invalid sig token: %v", err)
	}

	// 1. Authentication Tests
	t.Run("Auth_MissingToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader([]byte(`{}`)))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("Auth_ExpiredToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer "+expiredToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("Auth_InvalidSignature", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer "+invalidSigToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("Auth_UserRoleForbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer "+userToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden for user role, got %d", rec.Code)
		}
	})

	t.Run("Auth_AdminRoleForbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden for admin role, got %d", rec.Code)
		}
	})

	// 2. Functional: Create and Enqueue Notification for Delivery
	t.Run("Functional_SendRegisterCodeWithoutExternalUserID", func(t *testing.T) {
		reqBody := handlers.SendInternalNotificationRequest{
			TemplateKey:    "auth.register_code",
			Locale:         "ru",
			RecipientEmail: "newcomer@example.com",
			RecipientName:  "Алексей",
			Variables: map[string]any{
				"code": "773311",
			},
		}
		raw, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+serviceToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected status 202 Accepted, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		var resp handlers.DirectNotificationResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if resp.ID == "" {
			t.Fatal("expected non-empty notification ID")
		}
		if resp.DeliveryStatus != string(domain.DeliveryStatusPending) {
			t.Errorf("expected delivery status 'pending', got '%s'", resp.DeliveryStatus)
		}
		if resp.RecipientEmail != "newcomer@example.com" {
			t.Errorf("expected email 'newcomer@example.com', got '%s'", resp.RecipientEmail)
		}

		// Verify that delivery worker / service can deliver it
		deliverErr := deliverySvc.Deliver(context.Background(), resp.ID)
		if deliverErr != nil {
			t.Fatalf("failed to deliver notification with delivery service: %v", deliverErr)
		}

		if len(sender.sentMsgs) != 1 {
			t.Fatalf("expected 1 sent message, got %d", len(sender.sentMsgs))
		}
		lastMsg := sender.sentMsgs[len(sender.sentMsgs)-1]
		if lastMsg.To != "newcomer@example.com" {
			t.Errorf("expected recipient 'newcomer@example.com', got '%s'", lastMsg.To)
		}
		if !strings.Contains(lastMsg.Subject, "773311") {
			t.Errorf("expected subject to contain '773311', got '%s'", lastMsg.Subject)
		}
	})

	t.Run("Functional_SendPasswordRecoveryWithExternalUserIDAndSecurityAudit", func(t *testing.T) {
		secretToken := "secret-reset-token-xyz-999"
		recoveryURL := "https://gmhelper.com/recover?token=" + secretToken

		reqBody := handlers.SendInternalNotificationRequest{
			TemplateKey:    "auth.password_recovery",
			Locale:         "en",
			ExternalUserID: "usr-guid-12345",
			RecipientEmail: "user@example.com",
			RecipientName:  "Jane Doe",
			Variables: map[string]any{
				"recoveryLink": recoveryURL,
			},
		}
		raw, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+serviceToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected status 202 Accepted, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		var resp handlers.DirectNotificationResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if resp.ExternalUserID != "usr-guid-12345" {
			t.Errorf("expected externalUserId 'usr-guid-12345', got '%s'", resp.ExternalUserID)
		}

		// Verify security: Audit log must NOT contain plaintext secret token in activity logs
		for _, logEntry := range auditRepo.created {
			b, _ := json.Marshal(logEntry)
			if strings.Contains(string(b), secretToken) {
				t.Errorf("security violation: raw reset token found in audit log entry: %s", string(b))
			}
		}
	})

	// 3. Validation & Error Handling
	t.Run("Validation_MissingRequiredVariable", func(t *testing.T) {
		reqBody := handlers.SendInternalNotificationRequest{
			TemplateKey:    "auth.password_recovery",
			Locale:         "en",
			RecipientEmail: "user@example.com",
			Variables:      map[string]any{}, // missing "recoveryLink"
		}
		raw, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+serviceToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for missing required variable, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Validation_UnknownTemplate", func(t *testing.T) {
		reqBody := handlers.SendInternalNotificationRequest{
			TemplateKey:    "auth.unknown_tpl",
			Locale:         "en",
			RecipientEmail: "user@example.com",
		}
		raw, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+serviceToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for unknown template, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Validation_InactiveTemplate", func(t *testing.T) {
		reqBody := handlers.SendInternalNotificationRequest{
			TemplateKey:    "auth.draft_tpl",
			Locale:         "en",
			RecipientEmail: "user@example.com",
		}
		raw, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/notifications/send", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+serviceToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for inactive draft template, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}
