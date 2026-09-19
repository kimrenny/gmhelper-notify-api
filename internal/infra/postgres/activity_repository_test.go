package postgres

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/lib/pq"
)

func TestActivityLogRepository_CreateAndGetByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewActivityLogRepository(db)
	now := time.Now().UTC()

	actorID := "usr-100"
	actorName := "Admin Tester"
	actorRole := "Owner"
	targetName := "Campaign Launch"
	errMsg := "SMTP timeout"

	log := &domain.ActivityLog{
		ID:           "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		EventType:    "campaign.failed",
		ActorType:    domain.ActorTypeUser,
		ActorUserID:  &actorID,
		ActorName:    &actorName,
		ActorRole:    &actorRole,
		TargetType:   domain.TargetTypeCampaign,
		TargetID:     "cmp-200",
		TargetName:   &targetName,
		Status:       domain.ActivityStatusFailure,
		Summary:      "Failed to deliver campaign",
		Details:      json.RawMessage(`{"attempt":3,"recipientCount":150}`),
		ErrorMessage: &errMsg,
		CreatedAt:    now,
	}

	// 1. Create with all fields
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO activity_logs (
	id, event_type, actor_type, actor_user_id, actor_name, actor_role,
	target_type, target_id, target_name, status, summary, details, error_message, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`)).
		WithArgs(
			log.ID,
			log.EventType,
			log.ActorType,
			log.ActorUserID,
			log.ActorName,
			log.ActorRole,
			log.TargetType,
			log.TargetID,
			log.TargetName,
			log.Status,
			log.Summary,
			[]byte(log.Details),
			log.ErrorMessage,
			log.CreatedAt,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), log); err != nil {
		t.Fatalf("failed to create activity log: %v", err)
	}

	// 2. GetByID
	columns := []string{
		"id", "event_type", "actor_type", "actor_user_id", "actor_name", "actor_role",
		"target_type", "target_id", "target_name", "status", "summary", "details", "error_message", "created_at",
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, event_type, actor_type, actor_user_id, actor_name, actor_role,
       target_type, target_id, target_name, status, summary, details, error_message, created_at
FROM activity_logs
WHERE id = $1`)).
		WithArgs(log.ID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(
				log.ID, log.EventType, log.ActorType, log.ActorUserID, log.ActorName, log.ActorRole,
				log.TargetType, log.TargetID, log.TargetName, log.Status, log.Summary,
				[]byte(log.Details), log.ErrorMessage, log.CreatedAt,
			))

	fetched, err := repo.GetByID(context.Background(), log.ID)
	if err != nil {
		t.Fatalf("failed to fetch activity log: %v", err)
	}
	if fetched.ID != log.ID || fetched.EventType != log.EventType || fetched.Status != log.Status {
		t.Fatalf("unexpected fetched log: %+v", fetched)
	}
	if fetched.ActorUserID == nil || *fetched.ActorUserID != actorID {
		t.Fatalf("expected actor user ID %s, got %v", actorID, fetched.ActorUserID)
	}
	if fetched.ErrorMessage == nil || *fetched.ErrorMessage != errMsg {
		t.Fatalf("expected error message %s, got %v", errMsg, fetched.ErrorMessage)
	}
	if string(fetched.Details) != string(log.Details) {
		t.Fatalf("expected details %s, got %s", string(log.Details), string(fetched.Details))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestActivityLogRepository_CreateNullableAndAutoGenerate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewActivityLogRepository(db)

	systemLog := &domain.ActivityLog{
		EventType:  "direct.delivered",
		ActorType:  domain.ActorTypeSystem,
		TargetType: domain.TargetTypeDirectNotification,
		TargetID:   "dir-300",
		Status:     domain.ActivityStatusSuccess,
		Summary:    "Direct message sent successfully",
	}

	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO activity_logs (
	id, event_type, actor_type, actor_user_id, actor_name, actor_role,
	target_type, target_id, target_name, status, summary, details, error_message, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`)).
		WithArgs(
			sqlmock.AnyArg(), // generated UUID
			systemLog.EventType,
			systemLog.ActorType,
			nil, // actor_user_id
			nil, // actor_name
			nil, // actor_role
			systemLog.TargetType,
			systemLog.TargetID,
			nil, // target_name
			systemLog.Status,
			systemLog.Summary,
			[]byte("{}"),     // default details
			nil,              // error_message
			sqlmock.AnyArg(), // auto generated created_at
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), systemLog); err != nil {
		t.Fatalf("failed to create system activity log: %v", err)
	}

	if systemLog.ID == "" {
		t.Fatal("expected ID to be auto-generated")
	}
	if systemLog.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be populated")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestActivityLogRepository_ValidationAndErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewActivityLogRepository(db)

	// Nil entity
	if err := repo.Create(context.Background(), nil); err != domain.ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for nil log, got: %v", err)
	}

	// Invalid entity (missing summary)
	invalidLog := &domain.ActivityLog{
		EventType:  "template.updated",
		ActorType:  domain.ActorTypeUser,
		TargetType: domain.TargetTypeTemplate,
		TargetID:   "tmpl-1",
	}
	if err := repo.Create(context.Background(), invalidLog); err != domain.ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for invalid log, got: %v", err)
	}

	// Conflict error (duplicate ID 23505)
	validLog := &domain.ActivityLog{
		ID:         "act-dup",
		EventType:  "settings.updated",
		ActorType:  domain.ActorTypeUser,
		TargetType: domain.TargetTypeSettings,
		TargetID:   "global",
		Status:     domain.ActivityStatusSuccess,
		Summary:    "Updated default locale",
		CreatedAt:  time.Now().UTC(),
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO activity_logs`)).
		WithArgs(
			validLog.ID, validLog.EventType, validLog.ActorType, nil, nil, nil,
			validLog.TargetType, validLog.TargetID, nil, validLog.Status, validLog.Summary,
			[]byte("{}"), nil, validLog.CreatedAt,
		).
		WillReturnError(&pq.Error{Code: "23505"})

	if err := repo.Create(context.Background(), validLog); err != domain.ErrConflict {
		t.Fatalf("expected ErrConflict on duplicate ID, got: %v", err)
	}

	// Not found on GetByID
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type`)).
		WithArgs("nonexistent").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err = repo.GetByID(context.Background(), "nonexistent")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing ID, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestActivityLogRepository_List_PaginationAndDeterministicOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewActivityLogRepository(db)
	now := time.Now().UTC()

	// 1. Total count query
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM activity_logs`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// 2. Paginated rows ordered by created_at DESC, id DESC
	columns := []string{
		"id", "event_type", "actor_type", "actor_user_id", "actor_name", "actor_role",
		"target_type", "target_id", "target_name", "status", "summary", "details", "error_message", "created_at",
	}

	targetName1 := "Welcome"
	targetName2 := "Automation Rule 1"

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, event_type, actor_type, actor_user_id, actor_name, actor_role,
       target_type, target_id, target_name, status, summary, details, error_message, created_at
FROM activity_logs
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2`)).
		WithArgs(10, 0).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("act-2", "template.updated", "user", "usr-1", "Admin", "Owner", "template", "t-1", &targetName1, "success", "Updated template", []byte(`{"version":2}`), nil, now).
			AddRow("act-1", "automation.created", "user", "usr-1", "Admin", "Owner", "automation_rule", "r-1", &targetName2, "success", "Created rule", []byte(`{"schedule":"0 9 * * *"}`), nil, now.Add(-1*time.Minute)))

	items, total, err := repo.List(context.Background(), domain.ActivityLogFilter{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("failed to list activity logs: %v", err)
	}

	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != "act-2" || items[1].ID != "act-1" {
		t.Fatalf("unexpected order: first=%s, second=%s", items[0].ID, items[1].ID)
	}

	// 3. Test empty count early return
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM activity_logs`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	emptyItems, emptyTotal, err := repo.List(context.Background(), domain.ActivityLogFilter{Limit: 20})
	if err != nil {
		t.Fatalf("failed to list when empty: %v", err)
	}
	if emptyTotal != 0 || len(emptyItems) != 0 {
		t.Fatalf("expected 0 total and empty slice, got total=%d len=%d", emptyTotal, len(emptyItems))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}

func TestActivityLogRepository_List_WithFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}
	defer db.Close()

	repo := NewActivityLogRepository(db)
	now := time.Now().UTC()
	fromDate := now.Add(-24 * time.Hour)
	toDate := now

	eventType := "template.created"
	actorUserID := "usr-123"
	targetType := domain.TargetTypeTemplate
	targetID := "tpl-999"
	status := domain.ActivityStatusSuccess

	filter := domain.ActivityLogFilter{
		EventType:   &eventType,
		ActorUserID: &actorUserID,
		TargetType:  &targetType,
		TargetID:    &targetID,
		Status:      &status,
		FromDate:    &fromDate,
		ToDate:      &toDate,
		Limit:       15,
		Offset:      30,
	}

	// 1. Total count query with WHERE
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM activity_logs WHERE event_type = $1 AND actor_user_id = $2 AND target_type = $3 AND target_id = $4 AND status = $5 AND created_at >= $6 AND created_at <= $7`)).
		WithArgs(eventType, actorUserID, string(targetType), targetID, string(status), fromDate.UTC(), toDate.UTC()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// 2. Paginated rows with WHERE
	columns := []string{
		"id", "event_type", "actor_type", "actor_user_id", "actor_name", "actor_role",
		"target_type", "target_id", "target_name", "status", "summary", "details", "error_message", "created_at",
	}

	targetName := "Filtered Template"
	actorName := "Owner Admin"
	actorRole := "owner"

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, event_type, actor_type, actor_user_id, actor_name, actor_role,
       target_type, target_id, target_name, status, summary, details, error_message, created_at
FROM activity_logs WHERE event_type = $1 AND actor_user_id = $2 AND target_type = $3 AND target_id = $4 AND status = $5 AND created_at >= $6 AND created_at <= $7
ORDER BY created_at DESC, id DESC
LIMIT $8 OFFSET $9`)).
		WithArgs(eventType, actorUserID, string(targetType), targetID, string(status), fromDate.UTC(), toDate.UTC(), 15, 30).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("act-filtered-1", eventType, "user", &actorUserID, &actorName, &actorRole, string(targetType), targetID, &targetName, string(status), "Created template", []byte(`{"version":1}`), nil, now))

	items, total, err := repo.List(context.Background(), filter)
	if err != nil {
		t.Fatalf("failed to list activity logs with filters: %v", err)
	}

	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != "act-filtered-1" || items[0].EventType != eventType {
		t.Errorf("unexpected item: %+v", items[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unfulfilled expectations: %v", err)
	}
}
