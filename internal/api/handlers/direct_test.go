package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type mockDirectRepo struct {
	notifications map[string]*domain.DirectNotification
	createErr     error
	listErr       error
	getErr        error
}

func (m *mockDirectRepo) GetByID(ctx context.Context, id string) (*domain.DirectNotification, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	n, ok := m.notifications[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return n, nil
}

func (m *mockDirectRepo) Create(ctx context.Context, notification *domain.DirectNotification) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.notifications[notification.ID] = notification
	return nil
}

func (m *mockDirectRepo) CreateWithInitialAttempt(ctx context.Context, notification *domain.DirectNotification, attempt *domain.DeliveryAttempt) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.notifications[notification.ID] = notification
	return nil
}

func (m *mockDirectRepo) ListPending(ctx context.Context) ([]*domain.DirectNotification, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var list []*domain.DirectNotification
	for _, n := range m.notifications {
		if n.DeliveryStatus == domain.DeliveryStatusPending {
			list = append(list, n)
		}
	}
	return list, nil
}

func (m *mockDirectRepo) UpdateStatus(ctx context.Context, id string, status domain.DeliveryStatus, attempts int, lastAttemptAt, sentAt *time.Time, errorMessage string) error {
	n, ok := m.notifications[id]
	if !ok {
		return domain.ErrNotFound
	}
	n.DeliveryStatus = status
	n.AttemptsCount = attempts
	n.LastAttemptAt = lastAttemptAt
	n.SentAt = sentAt
	n.ErrorMessage = errorMessage
	return nil
}

type mockDirectAttemptRepo struct {
	attempts map[string]*domain.DeliveryAttempt
}

func (m *mockDirectAttemptRepo) GetByID(ctx context.Context, id string) (*domain.DeliveryAttempt, error) {
	att, ok := m.attempts[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return att, nil
}

func (m *mockDirectAttemptRepo) Create(ctx context.Context, attempt *domain.DeliveryAttempt) error {
	m.attempts[attempt.ID] = attempt
	return nil
}

func (m *mockDirectAttemptRepo) Update(ctx context.Context, attempt *domain.DeliveryAttempt) error {
	if _, ok := m.attempts[attempt.ID]; !ok {
		return domain.ErrNotFound
	}
	m.attempts[attempt.ID] = attempt
	return nil
}

func (m *mockDirectAttemptRepo) ListByTarget(ctx context.Context, targetType domain.DeliveryTargetType, targetID string) ([]*domain.DeliveryAttempt, error) {
	var list []*domain.DeliveryAttempt
	for _, att := range m.attempts {
		if att.TargetType == targetType && att.TargetID == targetID {
			list = append(list, att)
		}
	}
	return list, nil
}

type mockDirectTplRepo struct {
	templates map[string]*domain.EmailTemplate
}

func (m *mockDirectTplRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	t, ok := m.templates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return t, nil
}
func (m *mockDirectTplRepo) GetByKey(ctx context.Context, key string) (*domain.EmailTemplate, error) {
	return nil, domain.ErrNotFound
}
func (m *mockDirectTplRepo) Create(ctx context.Context, t *domain.EmailTemplate) error { return nil }
func (m *mockDirectTplRepo) Update(ctx context.Context, t *domain.EmailTemplate) error { return nil }
func (m *mockDirectTplRepo) Delete(ctx context.Context, id string) error               { return nil }
func (m *mockDirectTplRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

type mockDirectSender struct {
	sendErr error
}

func (m *mockDirectSender) Send(ctx context.Context, msg *email.Message) error {
	return m.sendErr
}

func setupDirectTestRouter() (http.Handler, *mockDirectRepo, *mockDirectAttemptRepo, *mockDirectTplRepo, *mockDirectSender) {
	log, _ := logger.NewLogger("info")
	directRepo := &mockDirectRepo{
		notifications: make(map[string]*domain.DirectNotification),
	}
	attemptRepo := &mockDirectAttemptRepo{
		attempts: make(map[string]*domain.DeliveryAttempt),
	}
	tplRepo := &mockDirectTplRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
	sender := &mockDirectSender{}

	directService := direct.NewService(tplRepo, directRepo)
	deliveryService := direct.NewDeliveryService(directRepo, attemptRepo, tplRepo, sender)
	handler := NewDirectNotificationHandler(directService, deliveryService, log)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/notifications/direct", handler.Create)
	mux.HandleFunc("GET /api/v1/notifications/direct/pending", handler.ListPending)
	mux.HandleFunc("GET /api/v1/notifications/direct/{id}", handler.GetByID)
	mux.HandleFunc("POST /api/v1/notifications/direct/{id}/deliver", handler.Deliver)

	return mux, directRepo, attemptRepo, tplRepo, sender
}

func TestDirectNotificationHandler_Create_Success(t *testing.T) {
	router, _, _, tplRepo, _ := setupDirectTestRouter()

	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-100",
		Subject:  "Welcome {{name}}",
		HTMLBody: "<p>Hi {{name}}</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	reqBody := CreateDirectNotificationRequest{
		TemplateID:     activeTpl.ID,
		RecipientEmail: "alice@example.com",
		RecipientName:  "Alice",
		Payload: map[string]any{
			"name": "Alice",
		},
	}
	raw, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp DirectNotificationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID == "" || resp.RecipientEmail != "alice@example.com" {
		t.Errorf("unexpected response content: %+v", resp)
	}
	if resp.DeliveryStatus != "pending" {
		t.Errorf("expected pending status, got %s", resp.DeliveryStatus)
	}
}

func TestDirectNotificationHandler_Create_ValidationErrors(t *testing.T) {
	router, _, _, tplRepo, _ := setupDirectTestRouter()

	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-100",
		Subject:  "Welcome {{name}}",
		HTMLBody: "<p>Hi {{name}}, code: {{code}}</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	draftTpl := &domain.EmailTemplate{
		ID:       "tpl-draft",
		Subject:  "Draft",
		HTMLBody: "<p>Draft</p>",
		Status:   domain.TemplateStatusDraft,
		Version:  1,
	}
	tplRepo.templates[draftTpl.ID] = draftTpl

	tests := []struct {
		name       string
		body       any
		rawJSON    string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "Empty Body",
			rawJSON:    "",
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "Malformed JSON",
			rawJSON:    "{invalid-json",
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Template Not Found",
			body: CreateDirectNotificationRequest{
				TemplateID:     "missing-tpl-id",
				RecipientEmail: "user@example.com",
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name: "Inactive Template",
			body: CreateDirectNotificationRequest{
				TemplateID:     draftTpl.ID,
				RecipientEmail: "user@example.com",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Invalid Recipient Email",
			body: CreateDirectNotificationRequest{
				TemplateID:     activeTpl.ID,
				RecipientEmail: "invalid-email-address",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Missing Template Variable",
			body: CreateDirectNotificationRequest{
				TemplateID:     activeTpl.ID,
				RecipientEmail: "user@example.com",
				Payload: map[string]any{
					"name": "Alice",
					// code is missing
				},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader *bytes.Reader
			if tt.rawJSON != "" {
				reader = bytes.NewReader([]byte(tt.rawJSON))
			} else {
				b, _ := json.Marshal(tt.body)
				reader = bytes.NewReader(b)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", reader)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			var errResp response.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			if errResp.Error.Code != tt.wantCode {
				t.Errorf("expected error code %s, got %s", tt.wantCode, errResp.Error.Code)
			}
		})
	}
}

func TestDirectNotificationHandler_GetByID_SuccessAndNotFound(t *testing.T) {
	router, directRepo, _, _, _ := setupDirectTestRouter()

	now := time.Now().UTC()
	notif := &domain.DirectNotification{
		ID:             "notif-100",
		TemplateID:     "tpl-100",
		RecipientEmail: "user@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	directRepo.notifications[notif.ID] = notif

	// 1. Found
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/notif-100", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp DirectNotificationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != "notif-100" {
		t.Errorf("expected ID notif-100, got %s", resp.ID)
	}

	// 2. Not Found
	reqMissing := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/non-existent", nil)
	recMissing := httptest.NewRecorder()
	router.ServeHTTP(recMissing, reqMissing)

	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recMissing.Code)
	}
}

func TestDirectNotificationHandler_ListPending(t *testing.T) {
	router, directRepo, _, _, _ := setupDirectTestRouter()

	// Empty list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/pending", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on empty list, got %d", rec.Code)
	}
	var list []DirectNotificationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}

	// Populate pending
	n1 := &domain.DirectNotification{
		ID:             "n-1",
		RecipientEmail: "a@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
	}
	n2 := &domain.DirectNotification{
		ID:             "n-2",
		RecipientEmail: "b@example.com",
		DeliveryStatus: domain.DeliveryStatusSent,
	}
	directRepo.notifications[n1.ID] = n1
	directRepo.notifications[n2.ID] = n2

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec2.Code)
	}
	var list2 []DirectNotificationResponse
	_ = json.Unmarshal(rec2.Body.Bytes(), &list2)
	if len(list2) != 1 || list2[0].ID != "n-1" {
		t.Errorf("expected exactly 1 pending item, got %d items", len(list2))
	}
}

func TestDirectNotificationHandler_Deliver_SuccessAndFailure(t *testing.T) {
	router, directRepo, attemptRepo, tplRepo, sender := setupDirectTestRouter()

	tpl := &domain.EmailTemplate{
		ID:       "tpl-1",
		Subject:  "Test",
		HTMLBody: "<p>Test</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[tpl.ID] = tpl

	notifSuccess := &domain.DirectNotification{
		ID:             "notif-success",
		TemplateID:     tpl.ID,
		RecipientEmail: "good@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
	}
	directRepo.notifications[notifSuccess.ID] = notifSuccess
	attemptRepo.attempts["att-1"] = &domain.DeliveryAttempt{
		ID:            "att-1",
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      notifSuccess.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
	}

	// 1. Deliver Success
	reqSuccess := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct/notif-success/deliver", nil)
	recSuccess := httptest.NewRecorder()
	router.ServeHTTP(recSuccess, reqSuccess)

	if recSuccess.Code != http.StatusOK {
		t.Fatalf("expected status 200 on successful deliver, got %d (body: %s)", recSuccess.Code, recSuccess.Body.String())
	}

	var respSuccess DirectNotificationResponse
	if err := json.Unmarshal(recSuccess.Body.Bytes(), &respSuccess); err != nil {
		t.Fatalf("failed to decode success response: %v", err)
	}
	if respSuccess.DeliveryStatus != "sent" {
		t.Errorf("expected sent delivery status, got %s", respSuccess.DeliveryStatus)
	}

	// 2. Deliver Already Sent (returns 400 Bad Request)
	recAlreadySent := httptest.NewRecorder()
	router.ServeHTTP(recAlreadySent, reqSuccess)
	if recAlreadySent.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 when delivering already sent notification, got %d", recAlreadySent.Code)
	}

	// 3. Deliver SMTP Failure (returns 500 Internal Server Error with standardized error response)
	notifFail := &domain.DirectNotification{
		ID:             "notif-fail",
		TemplateID:     tpl.ID,
		RecipientEmail: "bad@example.com",
		DeliveryStatus: domain.DeliveryStatusPending,
		AttemptsCount:  0,
	}
	directRepo.notifications[notifFail.ID] = notifFail

	sender.sendErr = errors.New("550 5.1.1 mailbox unavailable")
	reqFail := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct/notif-fail/deliver", nil)
	recFail := httptest.NewRecorder()
	router.ServeHTTP(recFail, reqFail)

	if recFail.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 on SMTP failure, got %d (body: %s)", recFail.Code, recFail.Body.String())
	}

	var errResp response.ErrorResponse
	if err := json.Unmarshal(recFail.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON error response: %v", err)
	}
	if errResp.Error.Code != "DELIVERY_FAILED" {
		t.Errorf("expected error code DELIVERY_FAILED, got %s", errResp.Error.Code)
	}

	// 4. Deliver Missing Notification (404)
	reqMissing := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct/non-existent-id/deliver", nil)
	recMissing := httptest.NewRecorder()
	router.ServeHTTP(recMissing, reqMissing)

	if recMissing.Code != http.StatusNotFound {
		t.Errorf("expected status 404 on missing notification delivery, got %d", recMissing.Code)
	}
}

func TestDirectNotificationHandler_Create_AuthenticatedPrincipalOverridesBodyUserID(t *testing.T) {
	router, _, _, tplRepo, _ := setupDirectTestRouter()

	activeTpl := &domain.EmailTemplate{
		ID:       "tpl-200",
		Subject:  "Hello",
		HTMLBody: "<p>Hello</p>",
		Status:   domain.TemplateStatusActive,
		Version:  1,
	}
	tplRepo.templates[activeTpl.ID] = activeTpl

	reqBody := CreateDirectNotificationRequest{
		TemplateID:     activeTpl.ID,
		ExternalUserID: "untrusted-client-user-id",
		RecipientEmail: "user@example.com",
	}
	raw, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(raw))
	// Inject verified principal into context
	ctx := middleware.ContextWithPrincipal(req.Context(), &domain.Principal{
		UserID: "verified-auth-user-999",
		Role:   "service",
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp DirectNotificationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ExternalUserID != "verified-auth-user-999" {
		t.Errorf("expected ExternalUserID 'verified-auth-user-999', got '%s'", resp.ExternalUserID)
	}
}
