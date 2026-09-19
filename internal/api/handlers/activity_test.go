package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/audit"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/response"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type handlerMockActivityRepo struct {
	logs    map[string]*domain.ActivityLog
	getErr  error
	listErr error
}

func newHandlerMockActivityRepo() *handlerMockActivityRepo {
	return &handlerMockActivityRepo{
		logs: make(map[string]*domain.ActivityLog),
	}
}

func (m *handlerMockActivityRepo) Create(ctx context.Context, log *domain.ActivityLog) error {
	m.logs[log.ID] = log
	return nil
}

func (m *handlerMockActivityRepo) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	log, ok := m.logs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return log, nil
}

func (m *handlerMockActivityRepo) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	var res []*domain.ActivityLog
	for _, l := range m.logs {
		if filter.EventType != nil && l.EventType != *filter.EventType {
			continue
		}
		if filter.ActorUserID != nil && (l.ActorUserID == nil || *l.ActorUserID != *filter.ActorUserID) {
			continue
		}
		if filter.TargetType != nil && l.TargetType != *filter.TargetType {
			continue
		}
		if filter.TargetID != nil && l.TargetID != *filter.TargetID {
			continue
		}
		if filter.Status != nil && l.Status != *filter.Status {
			continue
		}
		res = append(res, l)
	}
	return res, len(res), nil
}

func setupActivityHandlerTest() (*ActivityHandler, *handlerMockActivityRepo) {
	repo := newHandlerMockActivityRepo()
	svc := audit.NewService(repo)
	log, _ := logger.NewLogger("error")
	handler := NewActivityHandler(svc, log)
	return handler, repo
}

func TestActivityHandler_List_Success(t *testing.T) {
	handler, repo := setupActivityHandlerTest()

	actorID := "usr-1"
	actorName := "Alice Owner"
	actorRole := "owner"
	targetName := "Campaign 1"

	repo.logs["act-1"] = &domain.ActivityLog{
		ID:          "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		EventType:   "campaign.created",
		ActorType:   domain.ActorTypeUser,
		ActorUserID: &actorID,
		ActorName:   &actorName,
		ActorRole:   &actorRole,
		TargetType:  domain.TargetTypeCampaign,
		TargetID:    "cmp-100",
		TargetName:  &targetName,
		Status:      domain.ActivityStatusSuccess,
		Summary:     "Created campaign",
		Details:     json.RawMessage(`{"name":"Campaign 1"}`),
		CreatedAt:   time.Now().UTC(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res ActivityListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if res.Total != 1 || len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got total=%d len=%d", res.Total, len(res.Items))
	}
	if res.Limit != 10 || res.Offset != 0 {
		t.Errorf("expected limit 10, offset 0, got limit=%d offset=%d", res.Limit, res.Offset)
	}
	if res.Items[0].ID != "7c9e6679-7425-40de-944b-e07fc1f90ae7" || res.Items[0].Actor.Role == nil || *res.Items[0].Actor.Role != "owner" {
		t.Errorf("unexpected item content: %+v", res.Items[0])
	}
}

func TestActivityHandler_List_WithAllFilters(t *testing.T) {
	handler, repo := setupActivityHandlerTest()

	actorID := "usr-1"
	repo.logs["act-match"] = &domain.ActivityLog{
		ID:          "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		EventType:   "template.created",
		ActorType:   domain.ActorTypeUser,
		ActorUserID: &actorID,
		TargetType:  domain.TargetTypeTemplate,
		TargetID:    "tpl-1",
		Status:      domain.ActivityStatusSuccess,
		Summary:     "Created template",
		CreatedAt:   time.Now().UTC(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?eventType=template.created&actorUserId=usr-1&targetType=template&targetId=tpl-1&status=success&fromDate=2026-01-01&toDate=2026-12-31", nil)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res ActivityListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if res.Total != 1 {
		t.Errorf("expected 1 matching item, got %d", res.Total)
	}
}

func TestActivityHandler_List_ValidationErrors(t *testing.T) {
	handler, _ := setupActivityHandlerTest()

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "Negative limit -> 400",
			query:      "?limit=-5",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Limit above 100 -> 400",
			query:      "?limit=101",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Non-numeric limit -> 400",
			query:      "?limit=abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Negative offset -> 400",
			query:      "?offset=-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Non-numeric offset -> 400",
			query:      "?offset=xyz",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid targetType -> 400",
			query:      "?targetType=invalid_target",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid status -> 400",
			query:      "?status=invalid_status",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Malformed fromDate -> 400",
			query:      "?fromDate=invalid-date",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Malformed toDate -> 400",
			query:      "?toDate=invalid-date",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/activity"+tt.query, nil)
			rec := httptest.NewRecorder()
			handler.List(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			var errResp response.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			if errResp.Error.Code != "BAD_REQUEST" {
				t.Errorf("expected error code BAD_REQUEST, got %s", errResp.Error.Code)
			}
		})
	}
}

func TestActivityHandler_List_InternalError(t *testing.T) {
	handler, repo := setupActivityHandlerTest()
	repo.listErr = errors.New("db query error")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on repository error, got %d", rec.Code)
	}
}

func TestActivityHandler_GetByID_Success(t *testing.T) {
	handler, repo := setupActivityHandlerTest()

	validUUID := "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	actorID := "usr-1"
	repo.logs[validUUID] = &domain.ActivityLog{
		ID:          validUUID,
		EventType:   "template.created",
		ActorType:   domain.ActorTypeUser,
		ActorUserID: &actorID,
		TargetType:  domain.TargetTypeTemplate,
		TargetID:    "tpl-1",
		Status:      domain.ActivityStatusSuccess,
		Summary:     "Created email template",
		Details:     json.RawMessage(`{"subject":"Welcome"}`),
		CreatedAt:   time.Now().UTC(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity/"+validUUID, nil)
	req.SetPathValue("id", validUUID)
	rec := httptest.NewRecorder()
	handler.GetByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var res ActivityDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if res.ID != validUUID || res.Summary != "Created email template" {
		t.Errorf("unexpected detail response: %+v", res)
	}
	if string(res.Details) != `{"subject":"Welcome"}` {
		t.Errorf("unexpected details: %s", string(res.Details))
	}
}

func TestActivityHandler_GetByID_MalformedUUID(t *testing.T) {
	handler, _ := setupActivityHandlerTest()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity/not-a-valid-uuid", nil)
	req.SetPathValue("id", "not-a-valid-uuid")
	rec := httptest.NewRecorder()
	handler.GetByID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for malformed UUID, got %d", rec.Code)
	}

	var errResp response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Code != "BAD_REQUEST" {
		t.Errorf("expected error code BAD_REQUEST, got %s", errResp.Error.Code)
	}
}

func TestActivityHandler_GetByID_NotFound(t *testing.T) {
	handler, _ := setupActivityHandlerTest()

	missingUUID := "00000000-0000-0000-0000-000000000000"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity/"+missingUUID, nil)
	req.SetPathValue("id", missingUUID)
	rec := httptest.NewRecorder()
	handler.GetByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for nonexistent activity, got %d", rec.Code)
	}

	var errResp response.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected error code NOT_FOUND, got %s", errResp.Error.Code)
	}
}

func TestActivityHandler_GetByID_MissingID(t *testing.T) {
	handler, _ := setupActivityHandlerTest()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity/", nil)
	req.SetPathValue("id", "")
	rec := httptest.NewRecorder()
	handler.GetByID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty id, got %d", rec.Code)
	}
}

func TestActivityHandler_GetByID_InternalError(t *testing.T) {
	handler, repo := setupActivityHandlerTest()
	repo.getErr = errors.New("db query error")

	validUUID := "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity/"+validUUID, nil)
	req.SetPathValue("id", validUUID)
	rec := httptest.NewRecorder()
	handler.GetByID(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on db error, got %d", rec.Code)
	}
}
