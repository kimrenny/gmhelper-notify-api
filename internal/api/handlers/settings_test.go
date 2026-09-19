package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gmhelper/notify-api/internal/app/settings"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type mockSettingRepoForHandler struct {
	settings map[string]*domain.AppSetting
	listErr  error
	saveErr  error
}

func (m *mockSettingRepoForHandler) GetByKey(ctx context.Context, key string) (*domain.AppSetting, error) {
	if s, ok := m.settings[key]; ok {
		return s, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockSettingRepoForHandler) Save(ctx context.Context, setting *domain.AppSetting) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.settings[setting.Key] = setting
	return nil
}

func (m *mockSettingRepoForHandler) ListByCategory(ctx context.Context, category string) ([]*domain.AppSetting, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []*domain.AppSetting
	for _, s := range m.settings {
		if s.Category == category {
			res = append(res, s)
		}
	}
	return res, nil
}

func (m *mockSettingRepoForHandler) ListAll(ctx context.Context) ([]*domain.AppSetting, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []*domain.AppSetting
	for _, s := range m.settings {
		res = append(res, s)
	}
	return res, nil
}

func TestSettingsHandler_GetSettings_Success(t *testing.T) {
	repo := &mockSettingRepoForHandler{
		settings: map[string]*domain.AppSetting{
			settings.KeyDefaultFromName: {Key: settings.KeyDefaultFromName, Value: "GMHelper System", Category: settings.CategoryNotification},
			settings.KeyReplyToEmail:    {Key: settings.KeyReplyToEmail, Value: "support@gmhelper.com", Category: settings.CategoryNotification},
			settings.KeyDefaultLocale:   {Key: settings.KeyDefaultLocale, Value: "ua", Category: settings.CategoryNotification},
		},
	}
	service := settings.NewService(repo, nil)
	log, _ := logger.NewLogger("error")
	handler := NewSettingsHandler(service, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()

	handler.GetSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var res settings.AppSettingsDTO
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if res.DefaultFromName != "GMHelper System" || res.ReplyToEmail != "support@gmhelper.com" || res.DefaultLocale != "ua" {
		t.Errorf("unexpected settings response: %+v", res)
	}
}

func TestSettingsHandler_GetSettings_InternalError(t *testing.T) {
	repo := &mockSettingRepoForHandler{listErr: errors.New("db query failed")}
	service := settings.NewService(repo, nil)
	log, _ := logger.NewLogger("error")
	handler := NewSettingsHandler(service, log)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()

	handler.GetSettings(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}

	var errResp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode JSON error: %v", err)
	}
	errDetail := errResp["error"].(map[string]any)
	if errDetail["code"] != "INTERNAL_ERROR" {
		t.Errorf("expected INTERNAL_ERROR, got %v", errDetail["code"])
	}
}

func TestSettingsHandler_UpdateSettings_Success(t *testing.T) {
	repo := &mockSettingRepoForHandler{settings: make(map[string]*domain.AppSetting)}
	service := settings.NewService(repo, nil)
	log, _ := logger.NewLogger("error")
	handler := NewSettingsHandler(service, log)

	input := map[string]string{
		"defaultFromName": "Admin Team",
		"replyToEmail":    "admin@gmhelper.com",
		"defaultLocale":   "de",
	}
	bodyBytes, _ := json.Marshal(input)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(bodyBytes))
	rr := httptest.NewRecorder()

	handler.UpdateSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var res settings.AppSettingsDTO
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if res.DefaultFromName != "Admin Team" || res.ReplyToEmail != "admin@gmhelper.com" || res.DefaultLocale != "de" {
		t.Errorf("unexpected settings in response: %+v", res)
	}
}

func TestSettingsHandler_UpdateSettings_InvalidJSON(t *testing.T) {
	repo := &mockSettingRepoForHandler{settings: make(map[string]*domain.AppSetting)}
	service := settings.NewService(repo, nil)
	log, _ := logger.NewLogger("error")
	handler := NewSettingsHandler(service, log)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader([]byte("{invalid-json")))
	rr := httptest.NewRecorder()

	handler.UpdateSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", rr.Code)
	}
}

func TestSettingsHandler_UpdateSettings_ValidationErrors(t *testing.T) {
	repo := &mockSettingRepoForHandler{settings: make(map[string]*domain.AppSetting)}
	service := settings.NewService(repo, nil)
	log, _ := logger.NewLogger("error")
	handler := NewSettingsHandler(service, log)

	// Invalid email
	input := map[string]string{
		"replyToEmail": "not-an-email",
	}
	bodyBytes, _ := json.Marshal(input)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(bodyBytes))
	rr := httptest.NewRecorder()

	handler.UpdateSettings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", rr.Code)
	}

	var errResp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&errResp)
	errDetail := errResp["error"].(map[string]any)
	if errDetail["code"] != "INVALID_INPUT" {
		t.Errorf("expected INVALID_INPUT code, got %v", errDetail["code"])
	}
}

func TestSettingsHandler_UpdateSettings_InternalError(t *testing.T) {
	repo := &mockSettingRepoForHandler{
		settings: make(map[string]*domain.AppSetting),
		saveErr:  errors.New("db error"),
	}
	service := settings.NewService(repo, nil)
	log, _ := logger.NewLogger("error")
	handler := NewSettingsHandler(service, log)

	input := map[string]string{
		"defaultFromName": "Admin Team",
	}
	bodyBytes, _ := json.Marshal(input)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(bodyBytes))
	rr := httptest.NewRecorder()

	handler.UpdateSettings(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}
