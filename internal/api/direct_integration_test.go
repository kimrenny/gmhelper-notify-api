package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/api/handlers"
	"github.com/gmhelper/notify-api/internal/app/direct"
	"github.com/gmhelper/notify-api/internal/app/health"
	"github.com/gmhelper/notify-api/internal/app/template"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/auth"
	"github.com/gmhelper/notify-api/internal/infra/logger"
	"github.com/gmhelper/notify-api/internal/infra/postgres"
	infrasmtp "github.com/gmhelper/notify-api/internal/infra/smtp"
	"github.com/gmhelper/notify-api/internal/infra/smtp/testserver"
	"github.com/google/uuid"
)

const (
	intTestSecret   = "integration-test-secret-key-32-chars!"
	intTestIssuer   = "gmhelper-api"
	intTestAudience = "gmhelper-notify-api"
)

func getIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://notify_user:notify_password@localhost:5432/notify_db?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pgDB, err := postgres.NewPostgresDB(ctx, dbURL)
	if err != nil {
		if os.Getenv("NOTIFY_API_INTEGRATION") != "" {
			t.Fatalf("failed to connect to PostgreSQL for integration test: %v", err)
		}
		t.Skipf("skipping PostgreSQL integration test (database unreachable: %v)", err)
		return nil
	}

	if err := postgres.ApplyMigrations(ctx, pgDB.DB()); err != nil {
		pgDB.Close(ctx)
		t.Fatalf("failed to apply database migrations: %v", err)
	}

	return pgDB.DB()
}

func getTestAuthHeader(t *testing.T, userID string) string {
	t.Helper()
	token, err := auth.GenerateToken(intTestSecret, intTestIssuer, intTestAudience, userID, "service", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate test auth token: %v", err)
	}
	return "Bearer " + token
}

func setupIntegrationServer(
	t *testing.T,
	db *sql.DB,
	smtpServer *testserver.FakeSMTPServer,
) (http.Handler, *postgres.EmailTemplateRepository, *postgres.DirectNotificationRepository, *postgres.DeliveryAttemptRepository) {
	t.Helper()

	log, _ := logger.NewLogger("info")
	readinessService := health.NewReadinessService(&testPinger{db: db})
	healthHandler := handlers.NewHealthHandler(readinessService, log)

	templateRepo := postgres.NewEmailTemplateRepository(db)
	templateService := template.NewService(templateRepo)
	templateHandler := handlers.NewTemplateHandler(templateService, log)

	directRepo := postgres.NewDirectNotificationRepository(db)
	attemptRepo := postgres.NewDeliveryAttemptRepository(db)
	smtpClient := infrasmtp.NewClient(smtpServer.Host, smtpServer.Port, "", "", "no-reply@gmhelper.local")

	directService := direct.NewService(templateRepo, directRepo)
	deliveryService := direct.NewDeliveryService(directRepo, attemptRepo, templateRepo, smtpClient)
	directHandler := handlers.NewDirectNotificationHandler(directService, deliveryService, log)

	jwtVerifier := auth.NewJWTVerifier(intTestSecret, intTestIssuer, intTestAudience)
	authMiddleware := middleware.Authenticate(jwtVerifier, log)

	router := NewRouter(healthHandler, templateHandler, directHandler, authMiddleware)
	return router, templateRepo, directRepo, attemptRepo
}

type testPinger struct {
	db *sql.DB
}

func (p *testPinger) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func cleanupRecords(t *testing.T, db *sql.DB, notifIDs []string, tplIDs []string) {
	t.Helper()
	ctx := context.Background()

	for _, nid := range notifIDs {
		_, _ = db.ExecContext(ctx, `DELETE FROM delivery_attempts WHERE target_id = $1`, nid)
		_, _ = db.ExecContext(ctx, `DELETE FROM direct_notifications WHERE id = $1`, nid)
	}
	for _, tid := range tplIDs {
		_, _ = db.ExecContext(ctx, `DELETE FROM email_templates WHERE id = $1`, tid)
	}
}

func TestIntegration_DirectNotification_FullLifecycle_Success(t *testing.T) {
	db := getIntegrationDB(t)
	smtpServer := testserver.StartFakeSMTPServer(t)
	defer smtpServer.Close()

	router, templateRepo, _, _ := setupIntegrationServer(t, db, smtpServer)

	ctx := context.Background()
	createdNotifIDs := []string{}
	createdTplIDs := []string{}
	defer func() {
		cleanupRecords(t, db, createdNotifIDs, createdTplIDs)
	}()

	// 1. Create Active EmailTemplate in PostgreSQL
	activeTpl := &domain.EmailTemplate{
		ID:            uuid.NewString(),
		TemplateKey:   "welcome_e2e_" + uuid.NewString()[:8],
		Name:          "Welcome E2E",
		Subject:       "Welcome to GMHelper, {{name}}!",
		HTMLBody:      "<h1>Hello {{name}}</h1><p>Your activation code is {{code}}.</p>",
		PlainTextBody: "Hello {{name}}, your activation code is {{code}}.",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := templateRepo.Create(ctx, activeTpl); err != nil {
		t.Fatalf("failed to seed email template: %v", err)
	}
	createdTplIDs = append(createdTplIDs, activeTpl.ID)

	// 2. POST /api/v1/notifications/direct (Creation with valid Bearer Token)
	createReqBody := handlers.CreateDirectNotificationRequest{
		TemplateID:       activeTpl.ID,
		RecipientEmail:   "alice@example.com",
		RecipientName:    "Alice",
		NotificationType: "direct",
		Payload: map[string]any{
			"name": "Alice",
			"code": "XYZ-777",
		},
	}
	rawCreate, _ := json.Marshal(createReqBody)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(rawCreate))
	reqCreate.Header.Set("Authorization", getTestAuthHeader(t, "verified-user-100"))
	recCreate := httptest.NewRecorder()
	router.ServeHTTP(recCreate, reqCreate)

	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201 Created, got %d (body: %s)", recCreate.Code, recCreate.Body.String())
	}

	var createdResp handlers.DirectNotificationResponse
	if err := json.Unmarshal(recCreate.Body.Bytes(), &createdResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if createdResp.ID == "" {
		t.Fatal("expected non-empty notification ID")
	}
	createdNotifIDs = append(createdNotifIDs, createdResp.ID)

	if createdResp.DeliveryStatus != "pending" {
		t.Errorf("expected pending delivery status, got %s", createdResp.DeliveryStatus)
	}
	if createdResp.AttemptsCount != 0 {
		t.Errorf("expected attempts_count 0, got %d", createdResp.AttemptsCount)
	}
	if createdResp.ExternalUserID != "verified-user-100" {
		t.Errorf("expected ExternalUserID verified-user-100, got %s", createdResp.ExternalUserID)
	}

	// 3. Verify Database State after Creation
	var (
		dbStatus        string
		dbAttemptsCount int
		dbSentAt        *time.Time
		dbErrorMessage  string
		dbExternalUser  string
	)
	err := db.QueryRowContext(ctx, `SELECT delivery_status, attempts_count, sent_at, error_message, external_user_id FROM direct_notifications WHERE id = $1`, createdResp.ID).
		Scan(&dbStatus, &dbAttemptsCount, &dbSentAt, &dbErrorMessage, &dbExternalUser)
	if err != nil {
		t.Fatalf("failed to query created direct_notification: %v", err)
	}
	if dbStatus != "pending" {
		t.Errorf("expected db delivery_status pending, got %s", dbStatus)
	}
	if dbAttemptsCount != 0 {
		t.Errorf("expected db attempts_count 0, got %d", dbAttemptsCount)
	}
	if dbSentAt != nil {
		t.Errorf("expected db sent_at to be NULL, got %v", dbSentAt)
	}
	if dbExternalUser != "verified-user-100" {
		t.Errorf("expected external_user_id 'verified-user-100', got %s", dbExternalUser)
	}

	var (
		attemptStatus  string
		attemptNumber  int
		attemptError   string
		attemptedAtVal time.Time
	)
	err = db.QueryRowContext(ctx, `SELECT status, attempt_number, error_message, attempted_at FROM delivery_attempts WHERE target_id = $1`, createdResp.ID).
		Scan(&attemptStatus, &attemptNumber, &attemptError, &attemptedAtVal)
	if err != nil {
		t.Fatalf("failed to query initial delivery_attempt: %v", err)
	}
	if attemptStatus != "pending" {
		t.Errorf("expected initial delivery attempt status pending, got %s", attemptStatus)
	}
	if attemptNumber != 1 {
		t.Errorf("expected attempt_number 1, got %d", attemptNumber)
	}

	// 4. POST /api/v1/notifications/direct/{id}/deliver (Execution with valid token)
	reqDeliver := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct/"+createdResp.ID+"/deliver", nil)
	reqDeliver.Header.Set("Authorization", getTestAuthHeader(t, "verified-user-100"))
	recDeliver := httptest.NewRecorder()
	router.ServeHTTP(recDeliver, reqDeliver)

	if recDeliver.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK on delivery, got %d (body: %s)", recDeliver.Code, recDeliver.Body.String())
	}

	var deliveredResp handlers.DirectNotificationResponse
	if err := json.Unmarshal(recDeliver.Body.Bytes(), &deliveredResp); err != nil {
		t.Fatalf("failed to decode deliver response: %v", err)
	}
	if deliveredResp.DeliveryStatus != "sent" {
		t.Errorf("expected sent status, got %s", deliveredResp.DeliveryStatus)
	}
	if deliveredResp.SentAt == nil {
		t.Error("expected non-nil sent_at in deliver response")
	}

	// 5. Verify Real Database State after Delivery
	err = db.QueryRowContext(ctx, `SELECT delivery_status, attempts_count, sent_at, error_message FROM direct_notifications WHERE id = $1`, createdResp.ID).
		Scan(&dbStatus, &dbAttemptsCount, &dbSentAt, &dbErrorMessage)
	if err != nil {
		t.Fatalf("failed to query direct_notification after delivery: %v", err)
	}
	if dbStatus != "sent" {
		t.Errorf("expected db delivery_status sent, got %s", dbStatus)
	}
	if dbAttemptsCount != 1 {
		t.Errorf("expected db attempts_count 1, got %d", dbAttemptsCount)
	}
	if dbSentAt == nil {
		t.Error("expected db sent_at IS NOT NULL")
	}
	if dbErrorMessage != "" {
		t.Errorf("expected empty error message, got: %s", dbErrorMessage)
	}

	err = db.QueryRowContext(ctx, `SELECT status, attempt_number, error_message FROM delivery_attempts WHERE target_id = $1`, createdResp.ID).
		Scan(&attemptStatus, &attemptNumber, &attemptError)
	if err != nil {
		t.Fatalf("failed to query delivery_attempt after delivery: %v", err)
	}
	if attemptStatus != "sent" {
		t.Errorf("expected delivery attempt status sent, got %s", attemptStatus)
	}
	if attemptNumber != 1 {
		t.Errorf("expected attempt_number 1, got %d", attemptNumber)
	}
	if attemptError != "" {
		t.Errorf("expected empty attempt error_message, got: %s", attemptError)
	}

	// 6. Verify SMTP Received MIME Message
	messages := smtpServer.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected exactly 1 received SMTP message, got %d", len(messages))
	}
	received := messages[0]
	if len(received.To) != 1 || received.To[0] != "alice@example.com" {
		t.Errorf("expected recipient alice@example.com, got %v", received.To)
	}
	if received.Subject != "Welcome to GMHelper, Alice!" {
		t.Errorf("expected rendered subject 'Welcome to GMHelper, Alice!', got '%s'", received.Subject)
	}
	if !strings.Contains(received.HTMLBody, "<h1>Hello Alice</h1>") || !strings.Contains(received.HTMLBody, "XYZ-777") {
		t.Errorf("unexpected HTML body: %s", received.HTMLBody)
	}
	if !strings.Contains(received.TextBody, "Hello Alice, your activation code is XYZ-777.") {
		t.Errorf("unexpected Plain Text body: %s", received.TextBody)
	}

	// 7. GET /api/v1/notifications/direct/{id} (with valid token)
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/"+createdResp.ID, nil)
	reqGet.Header.Set("Authorization", getTestAuthHeader(t, "verified-user-100"))
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 on GetByID, got %d", recGet.Code)
	}
	var getResp handlers.DirectNotificationResponse
	_ = json.Unmarshal(recGet.Body.Bytes(), &getResp)
	if getResp.ID != createdResp.ID || getResp.DeliveryStatus != "sent" {
		t.Errorf("unexpected GetByID response: %+v", getResp)
	}
}

func TestIntegration_DirectNotification_Authentication_Security(t *testing.T) {
	db := getIntegrationDB(t)
	smtpServer := testserver.StartFakeSMTPServer(t)
	defer smtpServer.Close()

	router, templateRepo, _, _ := setupIntegrationServer(t, db, smtpServer)
	ctx := context.Background()

	createdTplIDs := []string{}
	defer func() {
		cleanupRecords(t, db, nil, createdTplIDs)
	}()

	activeTpl := &domain.EmailTemplate{
		ID:          uuid.NewString(),
		TemplateKey: "auth_sec_" + uuid.NewString()[:8],
		Name:        "Auth Security Tpl",
		Subject:     "Hello",
		HTMLBody:    "<p>Hello</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	_ = templateRepo.Create(ctx, activeTpl)
	createdTplIDs = append(createdTplIDs, activeTpl.ID)

	validBody, _ := json.Marshal(handlers.CreateDirectNotificationRequest{
		TemplateID:     activeTpl.ID,
		RecipientEmail: "alice@example.com",
	})

	// 1. GET /health without JWT -> 200 OK (Public)
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	router.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Errorf("expected 200 OK on public /health, got %d", recHealth.Code)
	}

	// 2. GET /ready without JWT -> 200 OK (Public)
	reqReady := httptest.NewRequest(http.MethodGet, "/ready", nil)
	recReady := httptest.NewRecorder()
	router.ServeHTTP(recReady, reqReady)
	if recReady.Code != http.StatusOK {
		t.Errorf("expected 200 OK on public /ready, got %d", recReady.Code)
	}

	// 3. POST /api/v1/notifications/direct without JWT -> 401
	reqNoAuth := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(validBody))
	recNoAuth := httptest.NewRecorder()
	router.ServeHTTP(recNoAuth, reqNoAuth)
	if recNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for missing auth header on POST /notifications/direct, got %d", recNoAuth.Code)
	}

	// 4. GET /api/v1/notifications/direct/pending without JWT -> 401
	reqPendingNoAuth := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/pending", nil)
	recPendingNoAuth := httptest.NewRecorder()
	router.ServeHTTP(recPendingNoAuth, reqPendingNoAuth)
	if recPendingNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for missing auth header on GET /pending, got %d", recPendingNoAuth.Code)
	}

	// 5. GET /api/v1/notifications/direct/{id} without JWT -> 401
	reqGetNoAuth := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/some-id", nil)
	recGetNoAuth := httptest.NewRecorder()
	router.ServeHTTP(recGetNoAuth, reqGetNoAuth)
	if recGetNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for missing auth header on GET /notifications/direct/{id}, got %d", recGetNoAuth.Code)
	}

	// 6. POST /api/v1/notifications/direct/{id}/deliver without JWT -> 401
	reqDeliverNoAuth := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct/some-id/deliver", nil)
	recDeliverNoAuth := httptest.NewRecorder()
	router.ServeHTTP(recDeliverNoAuth, reqDeliverNoAuth)
	if recDeliverNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for missing auth header on POST /deliver, got %d", recDeliverNoAuth.Code)
	}

	// 7. Token Signed with Wrong Secret -> 401
	badSecretToken, _ := auth.GenerateToken("completely-wrong-secret-key-32-chars", intTestIssuer, intTestAudience, "user-1", "service", time.Hour)
	reqBadSig := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(validBody))
	reqBadSig.Header.Set("Authorization", "Bearer "+badSecretToken)
	recBadSig := httptest.NewRecorder()
	router.ServeHTTP(recBadSig, reqBadSig)
	if recBadSig.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for bad signature, got %d", recBadSig.Code)
	}

	// 8. Expired Token -> 401
	expiredToken, _ := auth.GenerateToken(intTestSecret, intTestIssuer, intTestAudience, "user-1", "service", -10*time.Minute)
	reqExpired := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(validBody))
	reqExpired.Header.Set("Authorization", "Bearer "+expiredToken)
	recExpired := httptest.NewRecorder()
	router.ServeHTTP(recExpired, reqExpired)
	if recExpired.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for expired token, got %d", recExpired.Code)
	}

	// 9. Wrong Issuer -> 401
	wrongIssToken, _ := auth.GenerateToken(intTestSecret, "untrusted-external-issuer", intTestAudience, "user-1", "service", time.Hour)
	reqWrongIss := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(validBody))
	reqWrongIss.Header.Set("Authorization", "Bearer "+wrongIssToken)
	recWrongIss := httptest.NewRecorder()
	router.ServeHTTP(recWrongIss, reqWrongIss)
	if recWrongIss.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for wrong issuer, got %d", recWrongIss.Code)
	}

	// 10. Wrong Audience -> 401
	wrongAudToken, _ := auth.GenerateToken(intTestSecret, intTestIssuer, "wrong-audience", "user-1", "service", time.Hour)
	reqWrongAud := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(validBody))
	reqWrongAud.Header.Set("Authorization", "Bearer "+wrongAudToken)
	recWrongAud := httptest.NewRecorder()
	router.ServeHTTP(recWrongAud, reqWrongAud)
	if recWrongAud.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for wrong audience, got %d", recWrongAud.Code)
	}

	// 11. alg=none header attack -> 401
	noneHdr := `{"alg":"none","typ":"JWT"}`
	noneClaims := fmt.Sprintf(`{"sub":"attacker","iss":"%s","aud":"%s","exp":%d}`, intTestIssuer, intTestAudience, time.Now().Add(time.Hour).Unix())
	noneToken := strings.TrimRight(auth.EncodeBase64ForTest([]byte(noneHdr)), "=") + "." + strings.TrimRight(auth.EncodeBase64ForTest([]byte(noneClaims)), "=") + "."
	reqNone := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(validBody))
	reqNone.Header.Set("Authorization", "Bearer "+noneToken)
	recNone := httptest.NewRecorder()
	router.ServeHTTP(recNone, reqNone)
	if recNone.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for alg=none, got %d", recNone.Code)
	}

	// 12. Modified Payload / Tampered Claims without re-signing -> 401
	validToken, _ := auth.GenerateToken(intTestSecret, intTestIssuer, intTestAudience, "user-victim", "user", time.Hour)
	parts := strings.Split(validToken, ".")
	tamperedPayload := auth.EncodeBase64ForTest([]byte(fmt.Sprintf(`{"sub":"attacker-admin","iss":"%s","aud":"%s","exp":%d}`, intTestIssuer, intTestAudience, time.Now().Add(time.Hour).Unix())))
	tamperedToken := parts[0] + "." + strings.TrimRight(tamperedPayload, "=") + "." + parts[2]
	reqTampered := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(validBody))
	reqTampered.Header.Set("Authorization", "Bearer "+tamperedToken)
	recTampered := httptest.NewRecorder()
	router.ServeHTTP(recTampered, reqTampered)
	if recTampered.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for tampered payload, got %d", recTampered.Code)
	}

	// 13. Identity Spoofing Test: Client sends conflicting externalUserId in JSON body
	// The authenticated principal "authenticated-trusted-caller" MUST override "spoofed-user-id" in DB!
	spoofBody, _ := json.Marshal(handlers.CreateDirectNotificationRequest{
		TemplateID:     activeTpl.ID,
		ExternalUserID: "spoofed-attacker-identity",
		RecipientEmail: "alice@example.com",
	})
	reqSpoof := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(spoofBody))
	reqSpoof.Header.Set("Authorization", getTestAuthHeader(t, "authenticated-trusted-caller"))
	recSpoof := httptest.NewRecorder()
	router.ServeHTTP(recSpoof, reqSpoof)
	if recSpoof.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on authenticated creation, got %d (body: %s)", recSpoof.Code, recSpoof.Body.String())
	}

	var spoofResp handlers.DirectNotificationResponse
	_ = json.Unmarshal(recSpoof.Body.Bytes(), &spoofResp)
	if spoofResp.ExternalUserID != "authenticated-trusted-caller" {
		t.Errorf("expected ExternalUserID in response to be 'authenticated-trusted-caller', got %s", spoofResp.ExternalUserID)
	}

	// Verify real PostgreSQL record persistence
	var dbStoredUserID string
	err := db.QueryRowContext(ctx, `SELECT external_user_id FROM direct_notifications WHERE id = $1`, spoofResp.ID).Scan(&dbStoredUserID)
	if err != nil {
		t.Fatalf("failed to query db for spoof verification: %v", err)
	}
	if dbStoredUserID != "authenticated-trusted-caller" {
		t.Errorf("SECURITY FAILURE: expected db external_user_id to be 'authenticated-trusted-caller', but stored %s", dbStoredUserID)
	}

	// Clean up created direct notification
	cleanupRecords(t, db, []string{spoofResp.ID}, nil)
}

func TestIntegration_DirectNotification_Create_InvalidScenarios(t *testing.T) {
	db := getIntegrationDB(t)
	smtpServer := testserver.StartFakeSMTPServer(t)
	defer smtpServer.Close()

	router, templateRepo, _, _ := setupIntegrationServer(t, db, smtpServer)
	ctx := context.Background()

	createdTplIDs := []string{}
	defer func() {
		cleanupRecords(t, db, nil, createdTplIDs)
	}()

	activeTpl := &domain.EmailTemplate{
		ID:            uuid.NewString(),
		TemplateKey:   "active_key_" + uuid.NewString()[:8],
		Name:          "Active Tpl",
		Subject:       "Hello {{userName}}",
		HTMLBody:      "<p>Code: {{code}}</p>",
		PlainTextBody: "Code: {{code}}",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := templateRepo.Create(ctx, activeTpl); err != nil {
		t.Fatalf("failed to seed active template: %v", err)
	}
	createdTplIDs = append(createdTplIDs, activeTpl.ID)

	draftTpl := &domain.EmailTemplate{
		ID:          uuid.NewString(),
		TemplateKey: "draft_key_" + uuid.NewString()[:8],
		Name:        "Draft Tpl",
		Subject:     "Draft",
		HTMLBody:    "<p>Draft</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusDraft,
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := templateRepo.Create(ctx, draftTpl); err != nil {
		t.Fatalf("failed to seed draft template: %v", err)
	}
	createdTplIDs = append(createdTplIDs, draftTpl.ID)

	tests := []struct {
		name       string
		body       any
		rawJSON    string
		wantStatus int
		wantCode   string
	}{
		{
			name: "Missing Template ID",
			body: handlers.CreateDirectNotificationRequest{
				TemplateID:     "",
				RecipientEmail: "user@example.com",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Invalid Recipient Email",
			body: handlers.CreateDirectNotificationRequest{
				TemplateID:     activeTpl.ID,
				RecipientEmail: "not-an-email-address",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Nonexistent Template",
			body: handlers.CreateDirectNotificationRequest{
				TemplateID:     uuid.NewString(),
				RecipientEmail: "user@example.com",
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name: "Inactive Template",
			body: handlers.CreateDirectNotificationRequest{
				TemplateID:     draftTpl.ID,
				RecipientEmail: "user@example.com",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "Missing Template Variable",
			body: handlers.CreateDirectNotificationRequest{
				TemplateID:     activeTpl.ID,
				RecipientEmail: "user@example.com",
				Payload: map[string]any{
					"userName": "Bob",
					// "code" is missing
				},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "Malformed JSON",
			rawJSON:    `{"templateId": "test", bad json`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "Unknown JSON Fields",
			rawJSON:    `{"templateId": "` + activeTpl.ID + `", "recipientEmail": "user@example.com", "unknownField": 123}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r *bytes.Reader
			if tt.rawJSON != "" {
				r = bytes.NewReader([]byte(tt.rawJSON))
			} else {
				b, _ := json.Marshal(tt.body)
				r = bytes.NewReader(b)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", r)
			req.Header.Set("Authorization", getTestAuthHeader(t, "test-service-user"))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected HTTP %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
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

func TestIntegration_DirectNotification_ListPending(t *testing.T) {
	db := getIntegrationDB(t)
	smtpServer := testserver.StartFakeSMTPServer(t)
	defer smtpServer.Close()

	router, templateRepo, directRepo, _ := setupIntegrationServer(t, db, smtpServer)
	ctx := context.Background()

	createdNotifIDs := []string{}
	createdTplIDs := []string{}
	defer func() {
		cleanupRecords(t, db, createdNotifIDs, createdTplIDs)
	}()

	tpl := &domain.EmailTemplate{
		ID:          uuid.NewString(),
		TemplateKey: "tpl_pending_" + uuid.NewString()[:8],
		Name:        "Tpl Pending",
		Subject:     "Pending {{x}}",
		HTMLBody:    "<p>{{x}}</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := templateRepo.Create(ctx, tpl); err != nil {
		t.Fatalf("failed to seed template: %v", err)
	}
	createdTplIDs = append(createdTplIDs, tpl.ID)

	// Create 2 pending notifications
	for i := 1; i <= 2; i++ {
		n := &domain.DirectNotification{
			ID:               uuid.NewString(),
			TemplateID:       tpl.ID,
			RecipientEmail:   "pending@example.com",
			NotificationType: domain.NotificationTypeDirect,
			DeliveryStatus:   domain.DeliveryStatusPending,
			AttemptsCount:    0,
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
		}
		if err := directRepo.Create(ctx, n); err != nil {
			t.Fatalf("failed to create pending notification: %v", err)
		}
		createdNotifIDs = append(createdNotifIDs, n.ID)
	}

	// Create 1 sent notification
	sentNotif := &domain.DirectNotification{
		ID:               uuid.NewString(),
		TemplateID:       tpl.ID,
		RecipientEmail:   "sent@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusSent,
		AttemptsCount:    1,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	if err := directRepo.Create(ctx, sentNotif); err != nil {
		t.Fatalf("failed to create sent notification: %v", err)
	}
	createdNotifIDs = append(createdNotifIDs, sentNotif.ID)

	// GET /api/v1/notifications/direct/pending (authenticated)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/direct/pending", nil)
	req.Header.Set("Authorization", getTestAuthHeader(t, "test-user"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rec.Code)
	}

	var list []handlers.DirectNotificationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}

	foundPendingCount := 0
	for _, item := range list {
		if item.ID == createdNotifIDs[0] || item.ID == createdNotifIDs[1] {
			foundPendingCount++
		}
		if item.ID == sentNotif.ID {
			t.Errorf("sent notification %s should NOT be in pending list", sentNotif.ID)
		}
	}
	if foundPendingCount != 2 {
		t.Errorf("expected 2 created pending notifications to be present, found %d", foundPendingCount)
	}
}

func TestIntegration_DirectNotification_Delivery_SMTPFailure(t *testing.T) {
	db := getIntegrationDB(t)
	smtpServer := testserver.StartFakeSMTPServer(t)
	defer smtpServer.Close()

	// Reject recipient to force SMTP failure
	smtpServer.RejectRecipient = true

	router, templateRepo, _, _ := setupIntegrationServer(t, db, smtpServer)
	ctx := context.Background()

	createdNotifIDs := []string{}
	createdTplIDs := []string{}
	defer func() {
		cleanupRecords(t, db, createdNotifIDs, createdTplIDs)
	}()

	tpl := &domain.EmailTemplate{
		ID:            uuid.NewString(),
		TemplateKey:   "tpl_fail_" + uuid.NewString()[:8],
		Name:          "Tpl Fail",
		Subject:       "Fail {{user}}",
		HTMLBody:      "<p>Fail</p>",
		PlainTextBody: "Fail",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := templateRepo.Create(ctx, tpl); err != nil {
		t.Fatalf("failed to seed template: %v", err)
	}
	createdTplIDs = append(createdTplIDs, tpl.ID)

	// Create direct notification via API
	createBody := handlers.CreateDirectNotificationRequest{
		TemplateID:     tpl.ID,
		RecipientEmail: "reject@example.com",
		Payload:        map[string]any{"user": "Alice"},
	}
	rawCreate, _ := json.Marshal(createBody)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct", bytes.NewReader(rawCreate))
	reqCreate.Header.Set("Authorization", getTestAuthHeader(t, "user-alice"))
	recCreate := httptest.NewRecorder()
	router.ServeHTTP(recCreate, reqCreate)

	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201 Created, got %d (body: %s)", recCreate.Code, recCreate.Body.String())
	}

	var createdResp handlers.DirectNotificationResponse
	if err := json.Unmarshal(recCreate.Body.Bytes(), &createdResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	createdNotifIDs = append(createdNotifIDs, createdResp.ID)

	// Attempt delivery -> SMTP rejects recipient
	reqDeliver := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct/"+createdResp.ID+"/deliver", nil)
	reqDeliver.Header.Set("Authorization", getTestAuthHeader(t, "user-alice"))
	recDeliver := httptest.NewRecorder()
	router.ServeHTTP(recDeliver, reqDeliver)

	if recDeliver.Code != http.StatusInternalServerError {
		t.Fatalf("expected HTTP 500 on SMTP failure, got %d (body: %s)", recDeliver.Code, recDeliver.Body.String())
	}

	var errResp response.ErrorResponse
	if err := json.Unmarshal(recDeliver.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON error: %v", err)
	}
	if errResp.Error.Code != "DELIVERY_FAILED" {
		t.Errorf("expected error code DELIVERY_FAILED, got %s", errResp.Error.Code)
	}

	// Verify DB state for failed notification
	var (
		dbStatus       string
		dbAttempts     int
		dbSentAt       *time.Time
		dbErrorMessage string
	)
	_ = db.QueryRowContext(ctx, `SELECT delivery_status, attempts_count, sent_at, error_message FROM direct_notifications WHERE id = $1`, createdResp.ID).
		Scan(&dbStatus, &dbAttempts, &dbSentAt, &dbErrorMessage)

	if dbStatus != "failed" {
		t.Errorf("expected db delivery_status failed, got %s", dbStatus)
	}
	if dbAttempts != 1 {
		t.Errorf("expected db attempts_count 1, got %d", dbAttempts)
	}
	if dbSentAt != nil {
		t.Error("expected db sent_at to remain NULL on failure")
	}
	if dbErrorMessage == "" {
		t.Error("expected db error_message to be populated on failure")
	}

	// Verify DB state for failed attempt
	var (
		attStatus string
		attNumber int
		attError  string
	)
	_ = db.QueryRowContext(ctx, `SELECT status, attempt_number, error_message FROM delivery_attempts WHERE target_id = $1`, createdResp.ID).
		Scan(&attStatus, &attNumber, &attError)

	if attStatus != "failed" {
		t.Errorf("expected delivery attempt status failed, got %s", attStatus)
	}
	if attNumber != 1 {
		t.Errorf("expected attempt_number 1, got %d", attNumber)
	}
	if attError == "" {
		t.Error("expected attempt error_message to be populated on failure")
	}
}

func TestIntegration_DirectNotification_Delivery_InvalidStates(t *testing.T) {
	db := getIntegrationDB(t)
	smtpServer := testserver.StartFakeSMTPServer(t)
	defer smtpServer.Close()

	router, templateRepo, directRepo, _ := setupIntegrationServer(t, db, smtpServer)
	ctx := context.Background()

	createdNotifIDs := []string{}
	createdTplIDs := []string{}
	defer func() {
		cleanupRecords(t, db, createdNotifIDs, createdTplIDs)
	}()

	tpl := &domain.EmailTemplate{
		ID:          uuid.NewString(),
		TemplateKey: "tpl_states_" + uuid.NewString()[:8],
		Name:        "Tpl States",
		Subject:     "States",
		HTMLBody:    "<p>States</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := templateRepo.Create(ctx, tpl); err != nil {
		t.Fatalf("failed to seed template: %v", err)
	}
	createdTplIDs = append(createdTplIDs, tpl.ID)

	// 1. Already Sent Notification
	now := time.Now().UTC()
	sentNotif := &domain.DirectNotification{
		ID:               uuid.NewString(),
		TemplateID:       tpl.ID,
		RecipientEmail:   "sent@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusSent,
		AttemptsCount:    1,
		SentAt:           &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := directRepo.Create(ctx, sentNotif); err != nil {
		t.Fatalf("failed to seed sent notification: %v", err)
	}
	createdNotifIDs = append(createdNotifIDs, sentNotif.ID)

	reqSent := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct/"+sentNotif.ID+"/deliver", nil)
	reqSent.Header.Set("Authorization", getTestAuthHeader(t, "user-test"))
	recSent := httptest.NewRecorder()
	router.ServeHTTP(recSent, reqSent)

	if recSent.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 when delivering already sent notification, got %d", recSent.Code)
	}

	// 2. Cancelled Notification
	cancelledNotif := &domain.DirectNotification{
		ID:               uuid.NewString(),
		TemplateID:       tpl.ID,
		RecipientEmail:   "cancelled@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusCancelled,
		AttemptsCount:    0,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := directRepo.Create(ctx, cancelledNotif); err != nil {
		t.Fatalf("failed to seed cancelled notification: %v", err)
	}
	createdNotifIDs = append(createdNotifIDs, cancelledNotif.ID)

	reqCancelled := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/direct/"+cancelledNotif.ID+"/deliver", nil)
	reqCancelled.Header.Set("Authorization", getTestAuthHeader(t, "user-test"))
	recCancelled := httptest.NewRecorder()
	router.ServeHTTP(recCancelled, reqCancelled)

	if recCancelled.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 when delivering cancelled notification, got %d", recCancelled.Code)
	}
}

func TestIntegration_DirectNotification_BackgroundWorker_EndToEnd(t *testing.T) {
	db := getIntegrationDB(t)
	smtpServer := testserver.StartFakeSMTPServer(t)
	defer smtpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log, _ := logger.NewLogger("info")
	templateRepo := postgres.NewEmailTemplateRepository(db)
	directRepo := postgres.NewDirectNotificationRepository(db)
	attemptRepo := postgres.NewDeliveryAttemptRepository(db)
	smtpClient := infrasmtp.NewClient(smtpServer.Host, smtpServer.Port, "", "", "no-reply@gmhelper.local")
	deliveryService := direct.NewDeliveryService(directRepo, attemptRepo, templateRepo, smtpClient)

	createdTplIDs := []string{}
	createdNotifIDs := []string{}
	defer func() {
		cleanupRecords(t, db, createdNotifIDs, createdTplIDs)
	}()

	// 1. Seed active template
	tpl := &domain.EmailTemplate{
		ID:            uuid.NewString(),
		TemplateKey:   "worker_e2e_" + uuid.NewString()[:8],
		Name:          "Worker E2E",
		Subject:       "Worker Alert: {{subject}}",
		HTMLBody:      "<p>Worker body: {{body}}</p>",
		PlainTextBody: "Worker body: {{body}}",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := templateRepo.Create(ctx, tpl); err != nil {
		t.Fatalf("failed to seed template: %v", err)
	}
	createdTplIDs = append(createdTplIDs, tpl.ID)

	// 2. Seed pending direct notification with initial delivery attempt
	notif := &domain.DirectNotification{
		ID:               uuid.NewString(),
		TemplateID:       tpl.ID,
		RecipientEmail:   "worker-recipient@example.com",
		NotificationType: domain.NotificationTypeDirect,
		DeliveryStatus:   domain.DeliveryStatusPending,
		AttemptsCount:    0,
		Payload:          []byte(`{"subject":"Background Works","body":"Auto Delivered"}`),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	attempt := &domain.DeliveryAttempt{
		ID:            uuid.NewString(),
		TargetType:    domain.DeliveryTargetDirectNotification,
		TargetID:      notif.ID,
		Status:        domain.DeliveryStatusPending,
		AttemptNumber: 1,
		AttemptedAt:   time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}
	if err := directRepo.CreateWithInitialAttempt(ctx, notif, attempt); err != nil {
		t.Fatalf("failed to seed pending notification with attempt: %v", err)
	}
	createdNotifIDs = append(createdNotifIDs, notif.ID)

	// 3. Start background worker with 20ms polling interval
	worker := direct.NewWorker(directRepo, deliveryService, 20*time.Millisecond, log)
	workerDone := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(workerDone)
	}()

	// 4. Poll database until notification becomes 'sent' or timeout
	deadline := time.Now().Add(3 * time.Second)
	var (
		dbStatus        string
		dbAttemptsCount int
		dbSentAt        *time.Time
	)
	for time.Now().Before(deadline) {
		err := db.QueryRowContext(ctx, `SELECT delivery_status, attempts_count, sent_at FROM direct_notifications WHERE id = $1`, notif.ID).
			Scan(&dbStatus, &dbAttemptsCount, &dbSentAt)
		if err == nil && dbStatus == "sent" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	// Stop worker cleanly
	cancel()
	select {
	case <-workerDone:
		// Clean exit
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker did not exit cleanly")
	}

	// 5. Verify Database State
	if dbStatus != "sent" {
		t.Fatalf("expected direct notification status 'sent' after worker execution, got '%s'", dbStatus)
	}
	if dbAttemptsCount != 1 {
		t.Errorf("expected attempts_count 1, got %d", dbAttemptsCount)
	}
	if dbSentAt == nil {
		t.Error("expected sent_at IS NOT NULL")
	}

	var attStatus string
	err := db.QueryRowContext(context.Background(), `SELECT status FROM delivery_attempts WHERE target_id = $1`, notif.ID).Scan(&attStatus)
	if err != nil {
		t.Fatalf("failed to query delivery_attempt: %v", err)
	}
	if attStatus != "sent" {
		t.Errorf("expected delivery_attempt status 'sent', got '%s'", attStatus)
	}

	// 6. Verify SMTP Received Message
	messages := smtpServer.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected exactly 1 received message in fake SMTP server, got %d", len(messages))
	}
	if messages[0].Subject != "Worker Alert: Background Works" {
		t.Errorf("expected subject 'Worker Alert: Background Works', got '%s'", messages[0].Subject)
	}
	if !strings.Contains(messages[0].HTMLBody, "Auto Delivered") {
		t.Errorf("expected HTMLBody to contain 'Auto Delivered', got: %s", messages[0].HTMLBody)
	}
}
