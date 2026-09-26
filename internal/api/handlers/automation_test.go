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

	"github.com/gmhelper/notify-api/internal/app/automation"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

func intPtr(i int) *int {
	return &i
}

func sampleHandlerConfig() domain.AutomationRuleConfig {
	return domain.AutomationRuleConfig{
		Version: 1,
		Trigger: domain.TriggerUserRegistered,
		Schedule: domain.ScheduleConfig{
			Type:      domain.ScheduleTypeDaily,
			HourUTC:   intPtr(3),
			MinuteUTC: intPtr(0),
		},
		Conditions: domain.ConditionGroup{
			Operator: domain.GroupOperatorAll,
			Conditions: []domain.ConditionNode{
				{
					Item: &domain.ConditionItem{
						Field:    domain.FieldIsActive,
						Operator: domain.OperatorEquals,
						Value:    true,
					},
				},
			},
		},
		Action: domain.ActionConfig{
			Type:         domain.ActionTypeSendEmail,
			CooldownDays: intPtr(30),
		},
	}
}

type handlerMockAutomationRepo struct {
	rules     map[string]*domain.AutomationRule
	createErr error
	updateErr error
	deleteErr error
	getErr    error
	listErr   error
}

func newHandlerMockAutomationRepo() *handlerMockAutomationRepo {
	return &handlerMockAutomationRepo{
		rules: make(map[string]*domain.AutomationRule),
	}
}

func (m *handlerMockAutomationRepo) GetByID(ctx context.Context, id string) (*domain.AutomationRule, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	rule, ok := m.rules[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *rule
	return &cp, nil
}

func (m *handlerMockAutomationRepo) Create(ctx context.Context, rule *domain.AutomationRule) error {
	if m.createErr != nil {
		return m.createErr
	}
	cp := *rule
	m.rules[rule.ID] = &cp
	return nil
}

func (m *handlerMockAutomationRepo) Update(ctx context.Context, rule *domain.AutomationRule) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if _, ok := m.rules[rule.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *rule
	m.rules[rule.ID] = &cp
	return nil
}

func (m *handlerMockAutomationRepo) UpdateEvaluationTimes(ctx context.Context, id string, lastEvaluatedAt, nextEvaluationAt *time.Time) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	r, ok := m.rules[id]
	if !ok {
		return domain.ErrNotFound
	}
	r.LastEvaluatedAt = lastEvaluatedAt
	r.NextEvaluationAt = nextEvaluationAt
	return nil
}

func (m *handlerMockAutomationRepo) Delete(ctx context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.rules[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.rules, id)
	return nil
}

func (m *handlerMockAutomationRepo) List(ctx context.Context) ([]*domain.AutomationRule, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []*domain.AutomationRule
	for _, r := range m.rules {
		cp := *r
		res = append(res, &cp)
	}
	return res, nil
}

func (m *handlerMockAutomationRepo) ListEnabled(ctx context.Context) ([]*domain.AutomationRule, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []*domain.AutomationRule
	for _, r := range m.rules {
		if r.Enabled {
			cp := *r
			res = append(res, &cp)
		}
	}
	return res, nil
}

type handlerMockTemplateRepo struct {
	templates map[string]*domain.EmailTemplate
}

func newHandlerMockTemplateRepo() *handlerMockTemplateRepo {
	return &handlerMockTemplateRepo{
		templates: make(map[string]*domain.EmailTemplate),
	}
}

func (m *handlerMockTemplateRepo) GetByID(ctx context.Context, id string) (*domain.EmailTemplate, error) {
	tpl, ok := m.templates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return tpl, nil
}

func (m *handlerMockTemplateRepo) GetByKey(ctx context.Context, templateKey string) (*domain.EmailTemplate, error) {
	return nil, nil
}

func (m *handlerMockTemplateRepo) GetByKeyAndLocale(ctx context.Context, templateKey, locale string) (*domain.EmailTemplate, error) {
	for _, tpl := range m.templates {
		if tpl.TemplateKey == templateKey && (tpl.Locale == locale || locale == "") {
			return tpl, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *handlerMockTemplateRepo) Create(ctx context.Context, template *domain.EmailTemplate) error {
	return nil
}

func (m *handlerMockTemplateRepo) Update(ctx context.Context, template *domain.EmailTemplate) error {
	return nil
}

func (m *handlerMockTemplateRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *handlerMockTemplateRepo) List(ctx context.Context) ([]*domain.EmailTemplate, error) {
	return nil, nil
}

type handlerMockAutomationExecutionRepo struct {
	execs   map[string][]*domain.AutomationExecution
	listErr error
}

func newHandlerMockAutomationExecutionRepo() *handlerMockAutomationExecutionRepo {
	return &handlerMockAutomationExecutionRepo{
		execs: make(map[string][]*domain.AutomationExecution),
	}
}

func (m *handlerMockAutomationExecutionRepo) RecordExecution(ctx context.Context, exec *domain.AutomationExecution) error {
	return nil
}

func (m *handlerMockAutomationExecutionRepo) HasExecution(ctx context.Context, ruleID, eventID string) (bool, error) {
	return false, nil
}

func (m *handlerMockAutomationExecutionRepo) GetLastSuccessfulExecution(ctx context.Context, ruleID, recipientEmail string, externalUserID *string) (*domain.AutomationExecution, error) {
	return nil, nil
}

func (m *handlerMockAutomationExecutionRepo) ListByRuleID(ctx context.Context, ruleID string, limit, offset int) ([]*domain.AutomationExecution, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	all := m.execs[ruleID]
	total := len(all)
	if offset >= total {
		return []*domain.AutomationExecution{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (m *handlerMockAutomationExecutionRepo) ExecuteRuleAtomic(
	ctx context.Context,
	ruleID, eventID, recipientEmail string,
	externalUserID *string,
	cooldownDays *int,
	eventTime time.Time,
	notif *domain.DirectNotification,
	exec *domain.AutomationExecution,
) (string, error) {
	return "success", nil
}

func setupAutomationTest() (*AutomationHandler, *handlerMockAutomationRepo, *handlerMockTemplateRepo, *handlerMockAutomationExecutionRepo) {
	autoRepo := newHandlerMockAutomationRepo()
	tplRepo := newHandlerMockTemplateRepo()
	execRepo := newHandlerMockAutomationExecutionRepo()
	tplRepo.templates["tpl-valid-1"] = &domain.EmailTemplate{
		ID:           "tpl-valid-1",
		Name:         "Valid Template 1",
		TemplateType: domain.TemplateTypeAutomation,
	}
	tplRepo.templates["tpl-valid-2"] = &domain.EmailTemplate{
		ID:           "tpl-valid-2",
		Name:         "Valid Template 2",
		TemplateType: domain.TemplateTypeAutomation,
	}

	svc := automation.NewService(autoRepo, tplRepo, execRepo, nil)
	log, _ := logger.NewLogger("error")
	handler := NewAutomationHandler(svc, log)

	return handler, autoRepo, tplRepo, execRepo
}

func TestAutomationHandler_List(t *testing.T) {
	handler, autoRepo, _, _ := setupAutomationTest()

	now := time.Now().UTC()
	autoRepo.rules["00000000-0000-0000-0000-000000000001"] = &domain.AutomationRule{
		ID:         "00000000-0000-0000-0000-000000000001",
		Name:       "Rule 1",
		TemplateID: "tpl-valid-1",
		Enabled:    true,
		Config:     sampleHandlerConfig(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// 1. List success -> 200 OK
	req := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules", nil)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res []AutomationRuleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if len(res) != 1 || res[0].ID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("unexpected list response: %+v", res)
	}

	// 2. Service error -> 500 Internal Server Error
	autoRepo.listErr = errors.New("db error")
	recErr := httptest.NewRecorder()
	handler.List(recErr, req)
	if recErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on list error, got %d", recErr.Code)
	}
}

func TestAutomationHandler_GetByID(t *testing.T) {
	handler, autoRepo, _, _ := setupAutomationTest()

	now := time.Now().UTC()
	autoRepo.rules["00000000-0000-0000-0000-000000000001"] = &domain.AutomationRule{
		ID:         "00000000-0000-0000-0000-000000000001",
		Name:       "Welcome Rule",
		TemplateID: "tpl-valid-1",
		Enabled:    true,
		Config:     sampleHandlerConfig(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// 1. Get success -> 200 OK
	req := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/00000000-0000-0000-0000-000000000001", nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000001")
	rec := httptest.NewRecorder()
	handler.GetByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res AutomationRuleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if res.ID != "00000000-0000-0000-0000-000000000001" || res.Name != "Welcome Rule" || !res.Enabled {
		t.Fatalf("unexpected rule response: %+v", res)
	}

	// 2. Missing ID -> 400 Bad Request
	reqMissing := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/", nil)
	recMissing := httptest.NewRecorder()
	handler.GetByID(recMissing, reqMissing)
	if recMissing.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing ID, got %d", recMissing.Code)
	}

	// 3. Invalid UUID -> 400 Bad Request
	reqInvalidID := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/not-a-uuid", nil)
	reqInvalidID.SetPathValue("id", "not-a-uuid")
	recInvalidID := httptest.NewRecorder()
	handler.GetByID(recInvalidID, reqInvalidID)
	if recInvalidID.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", recInvalidID.Code)
	}

	// 4. Not Found -> 404 Not Found
	reqNotFound := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/00000000-0000-0000-0000-000000000404", nil)
	reqNotFound.SetPathValue("id", "00000000-0000-0000-0000-000000000404")
	recNotFound := httptest.NewRecorder()
	handler.GetByID(recNotFound, reqNotFound)
	if recNotFound.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent rule, got %d", recNotFound.Code)
	}

	// 4. Service error -> 500
	autoRepo.getErr = errors.New("db error")
	recDbErr := httptest.NewRecorder()
	handler.GetByID(recDbErr, req)
	if recDbErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on db error, got %d", recDbErr.Code)
	}
}

func TestAutomationHandler_Create(t *testing.T) {
	handler, autoRepo, _, _ := setupAutomationTest()

	// 1. Create success -> 201 Created
	enabled := true
	payload := CreateAutomationRuleRequest{
		Name:       "New User Welcome",
		TemplateID: "tpl-valid-1",
		Enabled:    &enabled,
		Config:     sampleHandlerConfig(),
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/automation/rules", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res AutomationRuleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if res.ID == "" || res.Name != "New User Welcome" || !res.Enabled {
		t.Fatalf("unexpected created rule: %+v", res)
	}

	// 2. Malformed JSON -> 400 Bad Request
	reqMalformed := httptest.NewRequest(http.MethodPost, "/api/v1/automation/rules", bytes.NewReader([]byte("{invalid-json")))
	recMalformed := httptest.NewRecorder()
	handler.Create(recMalformed, reqMalformed)
	if recMalformed.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed JSON, got %d", recMalformed.Code)
	}

	// 3. Validation failure (missing name) -> 400 Bad Request
	invalidPayload := CreateAutomationRuleRequest{
		Name:       "",
		TemplateID: "tpl-valid-1",
		Config:     sampleHandlerConfig(),
	}
	bodyInv, _ := json.Marshal(invalidPayload)
	reqInv := httptest.NewRequest(http.MethodPost, "/api/v1/automation/rules", bytes.NewReader(bodyInv))
	recInv := httptest.NewRecorder()
	handler.Create(recInv, reqInv)
	if recInv.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid payload, got %d", recInv.Code)
	}

	// 4. Template Not Found -> 400 Bad Request
	missingTplPayload := CreateAutomationRuleRequest{
		Name:       "Rule with missing template",
		TemplateID: "tpl-non-existent",
		Config:     sampleHandlerConfig(),
	}
	bodyMissingTpl, _ := json.Marshal(missingTplPayload)
	reqMissingTpl := httptest.NewRequest(http.MethodPost, "/api/v1/automation/rules", bytes.NewReader(bodyMissingTpl))
	recMissingTpl := httptest.NewRecorder()
	handler.Create(recMissingTpl, reqMissingTpl)
	if recMissingTpl.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing template, got %d", recMissingTpl.Code)
	}

	// 5. Service error -> 500
	autoRepo.createErr = errors.New("db insert error")
	reqSvcErr := httptest.NewRequest(http.MethodPost, "/api/v1/automation/rules", bytes.NewReader(body))
	recSvcErr := httptest.NewRecorder()
	handler.Create(recSvcErr, reqSvcErr)
	if recSvcErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on db error, got %d", recSvcErr.Code)
	}
}

func TestAutomationHandler_Update(t *testing.T) {
	handler, autoRepo, _, _ := setupAutomationTest()

	ruleID := "00000000-0000-0000-0000-000000000002"
	now := time.Now().UTC()
	autoRepo.rules[ruleID] = &domain.AutomationRule{
		ID:         ruleID,
		Name:       "Original Name",
		TemplateID: "tpl-valid-1",
		Enabled:    true,
		Config:     sampleHandlerConfig(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// 1. Update success -> 200 OK
	updatedName := "Updated Rule Name"
	updatedEnabled := false
	updatedTpl := "tpl-valid-2"
	updatePayload := UpdateAutomationRuleRequest{
		Name:       &updatedName,
		TemplateID: &updatedTpl,
		Enabled:    &updatedEnabled,
	}
	body, _ := json.Marshal(updatePayload)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/automation/rules/"+ruleID, bytes.NewReader(body))
	req.SetPathValue("id", ruleID)
	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res AutomationRuleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if res.Name != updatedName || res.TemplateID != updatedTpl || res.Enabled != false {
		t.Fatalf("unexpected updated response: %+v", res)
	}

	// 2. Missing ID -> 400 Bad Request
	reqMissingID := httptest.NewRequest(http.MethodPut, "/api/v1/automation/rules/", bytes.NewReader(body))
	recMissingID := httptest.NewRecorder()
	handler.Update(recMissingID, reqMissingID)
	if recMissingID.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing ID, got %d", recMissingID.Code)
	}

	// 3. Malformed JSON -> 400 Bad Request
	reqMalformed := httptest.NewRequest(http.MethodPut, "/api/v1/automation/rules/"+ruleID, bytes.NewReader([]byte("{invalid")))
	reqMalformed.SetPathValue("id", ruleID)
	recMalformed := httptest.NewRecorder()
	handler.Update(recMalformed, reqMalformed)
	if recMalformed.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed JSON, got %d", recMalformed.Code)
	}

	// 4. Validation failure (empty name) -> 400 Bad Request
	emptyName := "  "
	invPayload := UpdateAutomationRuleRequest{Name: &emptyName}
	bodyInv, _ := json.Marshal(invPayload)
	reqInv := httptest.NewRequest(http.MethodPut, "/api/v1/automation/rules/"+ruleID, bytes.NewReader(bodyInv))
	reqInv.SetPathValue("id", ruleID)
	recInv := httptest.NewRecorder()
	handler.Update(recInv, reqInv)
	if recInv.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty name, got %d", recInv.Code)
	}

	// 5. Template Not Found -> 400 Bad Request
	badTpl := "tpl-missing"
	badTplPayload := UpdateAutomationRuleRequest{TemplateID: &badTpl}
	bodyBadTpl, _ := json.Marshal(badTplPayload)
	reqBadTpl := httptest.NewRequest(http.MethodPut, "/api/v1/automation/rules/"+ruleID, bytes.NewReader(bodyBadTpl))
	reqBadTpl.SetPathValue("id", ruleID)
	recBadTpl := httptest.NewRecorder()
	handler.Update(recBadTpl, reqBadTpl)
	if recBadTpl.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing template in update, got %d", recBadTpl.Code)
	}

	// 6. Rule Not Found -> 404 Not Found
	reqNotFound := httptest.NewRequest(http.MethodPut, "/api/v1/automation/rules/00000000-0000-0000-0000-000000000404", bytes.NewReader(body))
	reqNotFound.SetPathValue("id", "00000000-0000-0000-0000-000000000404")
	recNotFound := httptest.NewRecorder()
	handler.Update(recNotFound, reqNotFound)
	if recNotFound.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not found rule, got %d", recNotFound.Code)
	}

	// 7. Service error -> 500
	autoRepo.updateErr = errors.New("db error")
	anotherName := "Another Name Changed"
	bodyErrPayload, _ := json.Marshal(UpdateAutomationRuleRequest{Name: &anotherName})
	reqErr := httptest.NewRequest(http.MethodPut, "/api/v1/automation/rules/"+ruleID, bytes.NewReader(bodyErrPayload))
	reqErr.SetPathValue("id", ruleID)
	recErr := httptest.NewRecorder()
	handler.Update(recErr, reqErr)
	if recErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on db error, got %d", recErr.Code)
	}
}

func TestAutomationHandler_Delete(t *testing.T) {
	handler, autoRepo, _, _ := setupAutomationTest()

	ruleID := "00000000-0000-0000-0000-000000000003"
	autoRepo.rules[ruleID] = &domain.AutomationRule{
		ID:         ruleID,
		Name:       "Rule To Delete",
		TemplateID: "tpl-valid-1",
		Enabled:    true,
		Config:     sampleHandlerConfig(),
	}

	// 1. Delete success -> 204 No Content
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/automation/rules/"+ruleID, nil)
	req.SetPathValue("id", ruleID)
	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if _, exists := autoRepo.rules[ruleID]; exists {
		t.Errorf("expected rule to be removed from repository")
	}

	// 2. Missing ID -> 400 Bad Request
	reqMissing := httptest.NewRequest(http.MethodDelete, "/api/v1/automation/rules/", nil)
	recMissing := httptest.NewRecorder()
	handler.Delete(recMissing, reqMissing)
	if recMissing.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing ID, got %d", recMissing.Code)
	}

	// 3. Not Found -> 404 Not Found
	reqNotFound := httptest.NewRequest(http.MethodDelete, "/api/v1/automation/rules/"+ruleID, nil)
	reqNotFound.SetPathValue("id", ruleID)
	recNotFound := httptest.NewRecorder()
	handler.Delete(recNotFound, reqNotFound)
	if recNotFound.Code != http.StatusNotFound {
		t.Errorf("expected 404 for deleted rule, got %d", recNotFound.Code)
	}

	// 4. Service error -> 500
	errRuleID := "00000000-0000-0000-0000-000000000004"
	autoRepo.rules[errRuleID] = &domain.AutomationRule{ID: errRuleID}
	autoRepo.deleteErr = errors.New("db error")
	reqErr := httptest.NewRequest(http.MethodDelete, "/api/v1/automation/rules/"+errRuleID, nil)
	reqErr.SetPathValue("id", errRuleID)
	recErr := httptest.NewRecorder()
	handler.Delete(recErr, reqErr)
	if recErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on db delete error, got %d", recErr.Code)
	}
}

func TestAutomationHandler_ListExecutions(t *testing.T) {
	handler, autoRepo, _, execRepo := setupAutomationTest()

	ruleID := "00000000-0000-0000-0000-000000000005"
	now := time.Now().UTC()
	uid := "u-123"
	notifID := "notif-abc"

	autoRepo.rules[ruleID] = &domain.AutomationRule{
		ID:         ruleID,
		Name:       "Execution History Test Rule",
		TemplateID: "tpl-valid-1",
		Enabled:    true,
		Config:     sampleHandlerConfig(),
	}

	execRepo.execs[ruleID] = []*domain.AutomationExecution{
		{
			ID:             "exec-1",
			RuleID:         ruleID,
			EventID:        "evt-100",
			RecipientEmail: "user1@example.com",
			ExternalUserID: &uid,
			NotificationID: &notifID,
			Status:         "success",
			ExecutedAt:     now,
			CreatedAt:      now,
		},
		{
			ID:             "exec-2",
			RuleID:         ruleID,
			EventID:        "evt-200",
			RecipientEmail: "user2@example.com",
			ExternalUserID: nil,
			NotificationID: nil,
			Status:         "skipped_cooldown",
			ExecutedAt:     now.Add(-10 * time.Minute),
			CreatedAt:      now.Add(-10 * time.Minute),
		},
	}

	// 1. Success 200 OK with paginated list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/"+ruleID+"/executions?limit=10&offset=0", nil)
	req.SetPathValue("id", ruleID)
	rec := httptest.NewRecorder()
	handler.ListExecutions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res AutomationExecutionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode execution response: %v", err)
	}
	if res.Total != 2 || len(res.Items) != 2 || res.Limit != 10 || res.Offset != 0 {
		t.Fatalf("unexpected execution list response: %+v", res)
	}
	if res.Items[0].ID != "exec-1" || res.Items[0].Status != "success" || *res.Items[0].ExternalUserID != "u-123" || *res.Items[0].NotificationID != "notif-abc" {
		t.Errorf("unexpected first item: %+v", res.Items[0])
	}
	if res.Items[1].ID != "exec-2" || res.Items[1].Status != "skipped_cooldown" {
		t.Errorf("unexpected second item: %+v", res.Items[1])
	}

	// 2. Empty history returns 200 OK with items: [], total: 0
	emptyRuleID := "00000000-0000-0000-0000-000000000006"
	autoRepo.rules[emptyRuleID] = &domain.AutomationRule{
		ID:         emptyRuleID,
		Name:       "Rule with no executions",
		TemplateID: "tpl-valid-1",
		Enabled:    true,
		Config:     sampleHandlerConfig(),
	}

	reqEmpty := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/"+emptyRuleID+"/executions", nil)
	reqEmpty.SetPathValue("id", emptyRuleID)
	recEmpty := httptest.NewRecorder()
	handler.ListExecutions(recEmpty, reqEmpty)

	if recEmpty.Code != http.StatusOK {
		t.Fatalf("expected status 200 for empty history, got %d", recEmpty.Code)
	}
	var resEmpty AutomationExecutionListResponse
	if err := json.Unmarshal(recEmpty.Body.Bytes(), &resEmpty); err != nil {
		t.Fatalf("failed to decode empty response: %v", err)
	}
	if resEmpty.Total != 0 || len(resEmpty.Items) != 0 {
		t.Errorf("expected total=0 and items=[], got total=%d, len=%d", resEmpty.Total, len(resEmpty.Items))
	}

	// 3. Invalid rule ID (not a UUID) -> 400 Bad Request
	reqInvalid := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/not-a-uuid/executions", nil)
	reqInvalid.SetPathValue("id", "not-a-uuid")
	recInvalid := httptest.NewRecorder()
	handler.ListExecutions(recInvalid, reqInvalid)
	if recInvalid.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-UUID rule id, got %d", recInvalid.Code)
	}

	// 4. Invalid limit -> 400 Bad Request
	reqBadLimit := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/"+ruleID+"/executions?limit=0", nil)
	reqBadLimit.SetPathValue("id", ruleID)
	recBadLimit := httptest.NewRecorder()
	handler.ListExecutions(recBadLimit, reqBadLimit)
	if recBadLimit.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for limit=0, got %d", recBadLimit.Code)
	}

	// 5. Invalid offset -> 400 Bad Request
	reqBadOffset := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/"+ruleID+"/executions?offset=-1", nil)
	reqBadOffset.SetPathValue("id", ruleID)
	recBadOffset := httptest.NewRecorder()
	handler.ListExecutions(recBadOffset, reqBadOffset)
	if recBadOffset.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for offset=-1, got %d", recBadOffset.Code)
	}

	// 6. Unknown rule ID -> 404 Not Found
	reqNotFound := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/00000000-0000-0000-0000-000000000999/executions", nil)
	reqNotFound.SetPathValue("id", "00000000-0000-0000-0000-000000000999")
	recNotFound := httptest.NewRecorder()
	handler.ListExecutions(recNotFound, reqNotFound)
	if recNotFound.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown rule id, got %d", recNotFound.Code)
	}

	// 7. Service error -> 500
	execRepo.listErr = errors.New("db failure")
	reqErr := httptest.NewRequest(http.MethodGet, "/api/v1/automation/rules/"+ruleID+"/executions", nil)
	reqErr.SetPathValue("id", ruleID)
	recErr := httptest.NewRecorder()
	handler.ListExecutions(recErr, reqErr)
	if recErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on execution repo failure, got %d", recErr.Code)
	}
}
