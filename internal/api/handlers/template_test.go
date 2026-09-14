package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/template"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type mockRepo struct {
	templates map[string]*domain.EmailTemplate
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	t, ok := m.templates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return t, nil
}

func (m *mockRepo) GetByKey(ctx context.Context, key string) (*domain.EmailTemplate, error) {
	for _, t := range m.templates {
		if t.TemplateKey == key {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockRepo) Create(ctx context.Context, t *domain.EmailTemplate) error {
	for _, existing := range m.templates {
		if existing.TemplateKey == t.TemplateKey && existing.Locale == t.Locale && existing.Version == t.Version {
			return domain.ErrConflict
		}
	}
	m.templates[t.ID] = t
	return nil
}

func (m *mockRepo) Update(ctx context.Context, t *domain.EmailTemplate) error {
	if _, ok := m.templates[t.ID]; !ok {
		return domain.ErrNotFound
	}
	for _, existing := range m.templates {
		if existing.ID != t.ID && existing.TemplateKey == t.TemplateKey && existing.Locale == t.Locale && existing.Version == t.Version {
			return domain.ErrConflict
		}
	}
	m.templates[t.ID] = t
	return nil
}

func (m *mockRepo) Delete(ctx context.Context, id string) error {
	if _, ok := m.templates[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.templates, id)
	return nil
}

func (m *mockRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	list := make([]*domain.EmailTemplate, 0, len(m.templates))
	for _, t := range m.templates {
		list = append(list, t)
	}
	return list, nil
}

func setupTestRouter(repo domain.EmailTemplateRepository) http.Handler {
	log, _ := logger.NewLogger("info")
	svc := template.NewService(repo)
	handler := NewTemplateHandler(svc, log)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/templates", handler.List)
	mux.HandleFunc("GET /api/v1/templates/{id}", handler.GetByID)
	mux.HandleFunc("POST /api/v1/templates", handler.Create)
	mux.HandleFunc("PUT /api/v1/templates/{id}", handler.Update)
	mux.HandleFunc("DELETE /api/v1/templates/{id}", handler.Delete)
	mux.HandleFunc("POST /api/v1/templates/{id}/preview", handler.Preview)
	return mux
}

func TestTemplateHandler_List(t *testing.T) {
	repo := newMockRepo()
	t1 := &domain.EmailTemplate{
		ID:          "1",
		TemplateKey: "welcome",
		Name:        "Welcome",
		Subject:     "Hello",
		HTMLBody:    "<p>Hello</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	repo.templates[t1.ID] = t1

	router := setupTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/templates", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var list []TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 template, got %d", len(list))
	}
}

func TestTemplateHandler_GetByID_SuccessAndNotFound(t *testing.T) {
	repo := newMockRepo()
	t1 := &domain.EmailTemplate{
		ID:          "tpl-100",
		TemplateKey: "verify_email",
		Name:        "Verify Email",
		Subject:     "Verify your email",
		HTMLBody:    "<p>Click here</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	repo.templates[t1.ID] = t1

	router := setupTestRouter(repo)

	// Existing
	req := httptest.NewRequest(http.MethodGet, "/api/v1/templates/tpl-100", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != "tpl-100" {
		t.Errorf("expected ID tpl-100, got %s", resp.ID)
	}

	// Not found
	reqMissing := httptest.NewRequest(http.MethodGet, "/api/v1/templates/nonexistent", nil)
	recMissing := httptest.NewRecorder()
	router.ServeHTTP(recMissing, reqMissing)

	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recMissing.Code)
	}

	var errResp response.ErrorResponse
	if err := json.Unmarshal(recMissing.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND code, got %s", errResp.Error.Code)
	}
}

func TestTemplateHandler_Create_Success(t *testing.T) {
	repo := newMockRepo()
	router := setupTestRouter(repo)

	payload := CreateTemplateRequest{
		TemplateKey:   "password_reset",
		Name:          "Password Reset",
		Subject:       "Reset Password",
		HTMLBody:      "<p>Reset link</p>",
		PlainTextBody: "Reset link text",
		Locale:        "en",
		Status:        "active",
		Version:       1,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.TemplateKey != "password_reset" {
		t.Errorf("expected template key password_reset, got %s", resp.TemplateKey)
	}
}

func TestTemplateHandler_Create_InvalidAndDuplicate(t *testing.T) {
	repo := newMockRepo()
	router := setupTestRouter(repo)

	// 1. Missing required field
	badPayload := map[string]any{
		"name": "No Key",
	}
	badBody, _ := json.Marshal(badPayload)
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/templates", bytes.NewReader(badBody))
	recBad := httptest.NewRecorder()
	router.ServeHTTP(recBad, reqBad)

	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recBad.Code)
	}

	var errResp response.ErrorResponse
	if err := json.Unmarshal(recBad.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Code != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST code, got %s", errResp.Error.Code)
	}

	// 2. Duplicate create -> 409 Conflict
	validPayload := CreateTemplateRequest{
		TemplateKey: "duplicate_key",
		Name:        "Name",
		Subject:     "Subject",
		HTMLBody:    "<p>body</p>",
		Locale:      "en",
		Version:     1,
	}
	validBody, _ := json.Marshal(validPayload)

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/templates", bytes.NewReader(validBody))
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on first create, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/templates", bytes.NewReader(validBody))
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected status 409 Conflict on duplicate create, got %d (body: %s)", rec2.Code, rec2.Body.String())
	}
	var conflictResp response.ErrorResponse
	_ = json.Unmarshal(rec2.Body.Bytes(), &conflictResp)
	if conflictResp.Error.Code != "CONFLICT" {
		t.Errorf("expected CONFLICT code, got %s", conflictResp.Error.Code)
	}
}

func TestTemplateHandler_Create_MalformedJSON(t *testing.T) {
	repo := newMockRepo()
	router := setupTestRouter(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates", bytes.NewReader([]byte("{invalid-json")))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestTemplateHandler_Update_SuccessNotFoundAndConflict(t *testing.T) {
	repo := newMockRepo()
	t1 := &domain.EmailTemplate{
		ID:          "tpl-1",
		TemplateKey: "key_1",
		Name:        "Name 1",
		Subject:     "Subject 1",
		HTMLBody:    "<p>1</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	t2 := &domain.EmailTemplate{
		ID:          "tpl-2",
		TemplateKey: "key_2",
		Name:        "Name 2",
		Subject:     "Subject 2",
		HTMLBody:    "<p>2</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	repo.templates[t1.ID] = t1
	repo.templates[t2.ID] = t2

	router := setupTestRouter(repo)

	// 1. Success update
	updatePayload := UpdateTemplateRequest{
		TemplateKey: "key_1_mod",
		Name:        "Name 1 Modified",
		Subject:     "Subject 1 Modified",
		HTMLBody:    "<p>modified</p>",
		Locale:      "en",
		Status:      "draft",
		Version:     1,
	}
	updateBody, _ := json.Marshal(updatePayload)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/templates/tpl-1", bytes.NewReader(updateBody))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// 2. Not found update
	reqMissing := httptest.NewRequest(http.MethodPut, "/api/v1/templates/tpl-999", bytes.NewReader(updateBody))
	recMissing := httptest.NewRecorder()
	router.ServeHTTP(recMissing, reqMissing)

	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recMissing.Code)
	}

	// 3. Conflict update (trying to update tpl-1 to key_2/en/v1 which is used by tpl-2)
	conflictPayload := UpdateTemplateRequest{
		TemplateKey: "key_2",
		Name:        "Name 1",
		Subject:     "Subject 1",
		HTMLBody:    "<p>body</p>",
		Locale:      "en",
		Version:     1,
	}
	conflictBody, _ := json.Marshal(conflictPayload)
	reqConflict := httptest.NewRequest(http.MethodPut, "/api/v1/templates/tpl-1", bytes.NewReader(conflictBody))
	recConflict := httptest.NewRecorder()
	router.ServeHTTP(recConflict, reqConflict)

	if recConflict.Code != http.StatusConflict {
		t.Fatalf("expected status 409 Conflict, got %d", recConflict.Code)
	}
}

func TestTemplateHandler_Delete_SuccessAndNotFound(t *testing.T) {
	repo := newMockRepo()
	t1 := &domain.EmailTemplate{
		ID:          "tpl-1",
		TemplateKey: "key_1",
		Name:        "Name 1",
		Subject:     "Subject 1",
		HTMLBody:    "<p>1</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
	}
	repo.templates[t1.ID] = t1

	router := setupTestRouter(repo)

	// 1. Successful delete -> 204 No Content
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/templates/tpl-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}

	// 2. Not found delete -> 404 Not Found
	reqMissing := httptest.NewRequest(http.MethodDelete, "/api/v1/templates/tpl-1", nil)
	recMissing := httptest.NewRecorder()
	router.ServeHTTP(recMissing, reqMissing)

	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recMissing.Code)
	}
}

func TestTemplateHandler_Preview_SuccessAndHTMLEscaping(t *testing.T) {
	repo := newMockRepo()
	tpl := &domain.EmailTemplate{
		ID:            "tpl-preview-1",
		TemplateKey:   "welcome_user",
		Name:          "Welcome Template",
		Subject:       "Welcome, {{username}}!",
		HTMLBody:      "<h1>Hello, {{username}}!</h1><p>Email: {{email}}</p>",
		PlainTextBody: "Hello, {{username}}!\nEmail: {{email}}",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	repo.templates[tpl.ID] = tpl

	router := setupTestRouter(repo)

	// Test payload with HTML special characters to verify escaping
	previewReq := PreviewTemplateRequest{
		Variables: map[string]any{
			"username": "<script>alert('xss')</script>",
			"email":    "john@example.com",
		},
	}
	body, _ := json.Marshal(previewReq)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates/tpl-preview-1/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp PreviewTemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// 1. Subject has raw variable substitution
	expectedSubject := "Welcome, <script>alert('xss')</script>!"
	if resp.Subject != expectedSubject {
		t.Errorf("expected subject '%s', got '%s'", expectedSubject, resp.Subject)
	}

	// 2. HTML body has HTML-escaped variable substitution
	expectedHTML := "<h1>Hello, &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;!</h1><p>Email: john@example.com</p>"
	if resp.HTMLBody != expectedHTML {
		t.Errorf("expected escaped HTML body '%s', got '%s'", expectedHTML, resp.HTMLBody)
	}

	// 3. Plain-text body has raw variable substitution
	expectedPlain := "Hello, <script>alert('xss')</script>!\nEmail: john@example.com"
	if resp.PlainTextBody != expectedPlain {
		t.Errorf("expected plain text '%s', got '%s'", expectedPlain, resp.PlainTextBody)
	}
}

func TestTemplateHandler_Preview_NotFound(t *testing.T) {
	repo := newMockRepo()
	router := setupTestRouter(repo)

	previewReq := PreviewTemplateRequest{
		Variables: map[string]any{"username": "John"},
	}
	body, _ := json.Marshal(previewReq)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates/nonexistent-id/preview", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	var errResp response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected error code NOT_FOUND, got %s", errResp.Error.Code)
	}
}

func TestTemplateHandler_Preview_MissingVariable(t *testing.T) {
	repo := newMockRepo()
	tpl := &domain.EmailTemplate{
		ID:          "tpl-preview-2",
		TemplateKey: "req_var_tpl",
		Name:        "Req Var Template",
		Subject:     "Subject {{code}}",
		HTMLBody:    "<p>Your code is: {{code}}</p>",
		Locale:      "en",
		Status:      domain.TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	repo.templates[tpl.ID] = tpl

	router := setupTestRouter(repo)

	// Missing "code" variable in payload
	previewReq := PreviewTemplateRequest{
		Variables: map[string]any{"other_var": "val"},
	}
	body, _ := json.Marshal(previewReq)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates/tpl-preview-2/preview", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request on missing template variable, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var errResp response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Code != "BAD_REQUEST" {
		t.Errorf("expected error code BAD_REQUEST, got %s", errResp.Error.Code)
	}
}

func TestTemplateHandler_Preview_InvalidJSONAndEmptyBody(t *testing.T) {
	repo := newMockRepo()
	router := setupTestRouter(repo)

	// 1. Invalid JSON
	reqBadJSON := httptest.NewRequest(http.MethodPost, "/api/v1/templates/tpl-1/preview", bytes.NewReader([]byte("{invalid-json")))
	recBadJSON := httptest.NewRecorder()
	router.ServeHTTP(recBadJSON, reqBadJSON)

	if recBadJSON.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 on bad JSON, got %d", recBadJSON.Code)
	}

	// 2. Empty Body
	reqEmpty := httptest.NewRequest(http.MethodPost, "/api/v1/templates/tpl-1/preview", bytes.NewReader([]byte("")))
	recEmpty := httptest.NewRecorder()
	router.ServeHTTP(recEmpty, reqEmpty)

	if recEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 on empty body, got %d", recEmpty.Code)
	}
}

func TestTemplateHandler_Preview_UnsavedOverrides(t *testing.T) {
	repo := newMockRepo()
	router := setupTestRouter(repo)

	subject := "Unsaved Subject {{name}}"
	htmlBody := "<p>Unsaved Body {{name}}</p>"
	plainText := "Unsaved Body {{name}}"

	previewReq := PreviewTemplateRequest{
		Subject:       &subject,
		HTMLBody:      &htmlBody,
		PlainTextBody: &plainText,
		Variables: map[string]any{
			"name": "Jane",
		},
	}
	body, _ := json.Marshal(previewReq)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates/new-template/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp PreviewTemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Subject != "Unsaved Subject Jane" {
		t.Errorf("expected subject 'Unsaved Subject Jane', got '%s'", resp.Subject)
	}
	if resp.HTMLBody != "<p>Unsaved Body Jane</p>" {
		t.Errorf("expected HTML body '<p>Unsaved Body Jane</p>', got '%s'", resp.HTMLBody)
	}
}

func TestTemplateHandler_Preview_ExistingTemplateIDWithOverrides(t *testing.T) {
	repo := newMockRepo()
	tpl := &domain.EmailTemplate{
		ID:            "tpl-persisted-1",
		TemplateKey:   "persisted_key",
		Name:          "Persisted Name",
		Subject:       "Persisted DB Subject",
		HTMLBody:      "<p>Persisted DB HTML</p>",
		PlainTextBody: "Persisted DB Plain",
		Locale:        "en",
		Status:        domain.TemplateStatusActive,
		Version:       1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	repo.templates[tpl.ID] = tpl

	router := setupTestRouter(repo)

	subject := "Live Unsaved Edited Subject {{name}}"
	htmlBody := "<div>Live Unsaved Edited Body {{name}}</div>"

	previewReq := PreviewTemplateRequest{
		Subject:  &subject,
		HTMLBody: &htmlBody,
		Variables: map[string]any{
			"name": "LiveUser",
		},
	}
	body, _ := json.Marshal(previewReq)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates/tpl-persisted-1/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp PreviewTemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Subject != "Live Unsaved Edited Subject LiveUser" {
		t.Errorf("expected overridden subject 'Live Unsaved Edited Subject LiveUser', got '%s'", resp.Subject)
	}
	if resp.HTMLBody != "<div>Live Unsaved Edited Body LiveUser</div>" {
		t.Errorf("expected overridden HTML body '<div>Live Unsaved Edited Body LiveUser</div>', got '%s'", resp.HTMLBody)
	}
	if resp.PlainTextBody != "Persisted DB Plain" {
		t.Errorf("expected fallback DB plain text 'Persisted DB Plain', got '%s'", resp.PlainTextBody)
	}
}

