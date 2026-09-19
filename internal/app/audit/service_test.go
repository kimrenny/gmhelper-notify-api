package audit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/domain"
	"github.com/gmhelper/notify-api/internal/http/middleware"
)

type mockActivityLogRepository struct {
	createFunc  func(ctx context.Context, log *domain.ActivityLog) error
	getByIDFunc func(ctx context.Context, id string) (*domain.ActivityLog, error)
	listFunc    func(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error)
}

func (m *mockActivityLogRepository) Create(ctx context.Context, log *domain.ActivityLog) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, log)
	}
	return nil
}

func (m *mockActivityLogRepository) GetByID(ctx context.Context, id string) (*domain.ActivityLog, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, domain.ErrNotFound
}

func (m *mockActivityLogRepository) List(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, filter)
	}
	return nil, 0, nil
}

func TestActor_ConstructorsAndValidation(t *testing.T) {
	userName := "Alice"
	userRole := "Admin"

	// 1. UserActor valid
	user := UserActor("usr-1", &userName, &userRole)
	if user.Type != domain.ActorTypeUser || user.UserID == nil || *user.UserID != "usr-1" {
		t.Fatalf("unexpected user actor: %+v", user)
	}
	if err := user.Validate(); err != nil {
		t.Fatalf("expected valid user actor, got: %v", err)
	}

	// 2. UserActor with empty ID
	invalidUser := UserActor("   ", nil, nil)
	if err := invalidUser.Validate(); !errors.Is(err, ErrInvalidActor) {
		t.Fatalf("expected ErrInvalidActor for empty user ID, got: %v", err)
	}

	// 3. SystemActor
	system := SystemActor()
	if system.Type != domain.ActorTypeSystem || system.UserID != nil || system.Name != nil || system.Role != nil {
		t.Fatalf("unexpected system actor: %+v", system)
	}
	if err := system.Validate(); err != nil {
		t.Fatalf("expected valid system actor, got: %v", err)
	}

	// 4. ServiceActor
	svcName := "billing-service"
	service := ServiceActor(&svcName)
	if service.Type != domain.ActorTypeService || service.UserID != nil || service.Name == nil || *service.Name != svcName {
		t.Fatalf("unexpected service actor: %+v", service)
	}
	if err := service.Validate(); err != nil {
		t.Fatalf("expected valid service actor, got: %v", err)
	}

	// 5. System actor with illegal user ID
	uid := "usr-999"
	illegalSystem := Actor{
		Type:   domain.ActorTypeSystem,
		UserID: &uid,
	}
	if err := illegalSystem.Validate(); !errors.Is(err, ErrInvalidActor) {
		t.Fatalf("expected ErrInvalidActor for system actor with user ID, got: %v", err)
	}
}

func TestActorFromContext(t *testing.T) {
	displayName := "Bob"

	// 1. Nil context -> returns ErrMissingPrincipal, not SystemActor
	actor, err := ActorFromContext(nil, &displayName)
	if !errors.Is(err, ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal for nil context, got err=%v", err)
	}
	if actor.Type == domain.ActorTypeSystem {
		t.Fatal("expected nil context to NOT return SystemActor")
	}

	// 2. Context without principal -> returns ErrMissingPrincipal, not SystemActor
	actor, err = ActorFromContext(context.Background(), &displayName)
	if !errors.Is(err, ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal for unauthenticated context, got err=%v", err)
	}
	if actor.Type == domain.ActorTypeSystem {
		t.Fatal("expected unauthenticated context to NOT return SystemActor")
	}

	// 3. Context with principal with empty UserID -> returns ErrMissingPrincipal
	emptyPrincipal := &domain.Principal{UserID: "  ", Role: "Admin"}
	ctxEmpty := middleware.ContextWithPrincipal(context.Background(), emptyPrincipal)
	actor, err = ActorFromContext(ctxEmpty, &displayName)
	if !errors.Is(err, ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal for empty UserID principal, got err=%v", err)
	}

	// 4. Context with valid principal -> UserActor
	principal := &domain.Principal{
		UserID: "usr-42",
		Role:   "Owner",
	}
	ctx := middleware.ContextWithPrincipal(context.Background(), principal)
	actor, err = ActorFromContext(ctx, &displayName)
	if err != nil {
		t.Fatalf("expected successful user actor extraction, got err=%v", err)
	}
	if actor.Type != domain.ActorTypeUser || actor.UserID == nil || *actor.UserID != "usr-42" {
		t.Fatalf("expected UserActor from context, got: %+v", actor)
	}
	if actor.Role == nil || *actor.Role != "Owner" {
		t.Fatalf("expected role Owner, got: %v", actor.Role)
	}
	if actor.Name == nil || *actor.Name != "Bob" {
		t.Fatalf("expected name Bob, got: %v", actor.Name)
	}
}

func TestUserActorFromPrincipal(t *testing.T) {
	name := "Alice"

	// 1. Nil principal -> returns ErrMissingPrincipal
	_, err := UserActorFromPrincipal(nil, &name)
	if !errors.Is(err, ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal for nil principal, got: %v", err)
	}

	// 2. Principal with empty user ID -> returns ErrMissingPrincipal
	_, err = UserActorFromPrincipal(&domain.Principal{UserID: ""}, &name)
	if !errors.Is(err, ErrMissingPrincipal) {
		t.Fatalf("expected ErrMissingPrincipal for empty UserID, got: %v", err)
	}

	// 3. Valid principal -> returns UserActor
	actor, err := UserActorFromPrincipal(&domain.Principal{UserID: "usr-1", Role: "Admin"}, &name)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if actor.Type != domain.ActorTypeUser || *actor.UserID != "usr-1" || *actor.Role != "Admin" || *actor.Name != "Alice" {
		t.Fatalf("unexpected actor: %+v", actor)
	}
}

func TestAuditService_Record_UserActorAndDetails(t *testing.T) {
	var capturedLog *domain.ActivityLog
	mockRepo := &mockActivityLogRepository{
		createFunc: func(ctx context.Context, log *domain.ActivityLog) error {
			capturedLog = log
			log.ID = "act-101"
			return nil
		},
	}

	service := NewService(mockRepo)
	userName := "Alice"
	userRole := "Admin"
	targetName := "Welcome Template"
	customTime := time.Date(2026, 9, 18, 22, 0, 0, 0, time.UTC)

	type TemplateDetails struct {
		Version int    `json:"version"`
		Locale  string `json:"locale"`
	}

	input := RecordInput{
		EventType:  domain.EventTemplateCreated,
		Actor:      UserActor("usr-1", &userName, &userRole),
		TargetType: domain.TargetTypeTemplate,
		TargetID:   "tmpl-1",
		TargetName: &targetName,
		Status:     domain.ActivityStatusSuccess,
		Summary:    "Created template Welcome",
		CreatedAt:  customTime,
		Details: TemplateDetails{
			Version: 1,
			Locale:  "en",
		},
	}

	created, err := service.Record(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful record, got: %v", err)
	}

	if created.ID != "act-101" {
		t.Fatalf("expected ID act-101, got: %s", created.ID)
	}
	if capturedLog.EventType != domain.EventTemplateCreated {
		t.Fatalf("expected event type %s, got: %s", domain.EventTemplateCreated, capturedLog.EventType)
	}
	if capturedLog.ActorType != domain.ActorTypeUser || *capturedLog.ActorUserID != "usr-1" {
		t.Fatalf("unexpected captured actor: %+v", capturedLog)
	}
	if *capturedLog.TargetName != "Welcome Template" {
		t.Fatalf("expected target name Welcome Template, got: %v", capturedLog.TargetName)
	}
	if !capturedLog.CreatedAt.Equal(customTime) {
		t.Fatalf("expected created at %v, got %v", customTime, capturedLog.CreatedAt)
	}

	var detailsMap map[string]any
	if err := json.Unmarshal(capturedLog.Details, &detailsMap); err != nil {
		t.Fatalf("failed to unmarshal captured details: %v", err)
	}
	if detailsMap["locale"] != "en" || detailsMap["version"] != float64(1) {
		t.Fatalf("unexpected details content: %+v", detailsMap)
	}
}

func TestAuditService_Record_SystemActor_NullableFields(t *testing.T) {
	var capturedLog *domain.ActivityLog
	mockRepo := &mockActivityLogRepository{
		createFunc: func(ctx context.Context, log *domain.ActivityLog) error {
			capturedLog = log
			return nil
		},
	}

	service := NewService(mockRepo)

	input := RecordInput{
		EventType:  domain.EventCampaignCompleted,
		Actor:      SystemActor(),
		TargetType: domain.TargetTypeCampaign,
		TargetID:   "cmp-500",
		Summary:    "Campaign finished delivery",
		Details: map[string]any{
			"sentCount":   100,
			"failedCount": 0,
		},
	}

	_, err := service.Record(context.Background(), input)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if capturedLog.ActorType != domain.ActorTypeSystem {
		t.Fatalf("expected actor type system, got: %s", capturedLog.ActorType)
	}
	if capturedLog.ActorUserID != nil {
		t.Fatalf("expected nil actor user ID for system actor, got: %v", capturedLog.ActorUserID)
	}
	if capturedLog.TargetName != nil {
		t.Fatalf("expected nil target name when omitted, got: %v", capturedLog.TargetName)
	}
	if capturedLog.ErrorMessage != nil {
		t.Fatalf("expected nil error message when omitted, got: %v", capturedLog.ErrorMessage)
	}
	if capturedLog.Status != domain.ActivityStatusSuccess {
		t.Fatalf("expected default status success, got: %s", capturedLog.Status)
	}
}

func TestAuditService_Record_FailureWithErrorMessage(t *testing.T) {
	var capturedLog *domain.ActivityLog
	mockRepo := &mockActivityLogRepository{
		createFunc: func(ctx context.Context, log *domain.ActivityLog) error {
			capturedLog = log
			return nil
		},
	}

	service := NewService(mockRepo)
	errMsg := "SMTP connection refused"

	input := RecordInput{
		EventType:    domain.EventDirectFailed,
		Actor:        SystemActor(),
		TargetType:   domain.TargetTypeDirectNotification,
		TargetID:     "dir-100",
		Status:       domain.ActivityStatusFailure,
		Summary:      "Direct message delivery failed",
		ErrorMessage: &errMsg,
		Details:      json.RawMessage(`{"attempt":5}`),
	}

	_, err := service.Record(context.Background(), input)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if capturedLog.Status != domain.ActivityStatusFailure {
		t.Fatalf("expected failure status, got: %s", capturedLog.Status)
	}
	if capturedLog.ErrorMessage == nil || *capturedLog.ErrorMessage != errMsg {
		t.Fatalf("expected error message %s, got: %v", errMsg, capturedLog.ErrorMessage)
	}
}

func TestAuditService_Record_StructuredDetailsTypes(t *testing.T) {
	mockRepo := &mockActivityLogRepository{}
	service := NewService(mockRepo)

	baseInput := func(details any) RecordInput {
		return RecordInput{
			EventType:  domain.EventSettingsUpdated,
			Actor:      SystemActor(),
			TargetType: domain.TargetTypeSettings,
			TargetID:   "global",
			Summary:    "Updated settings",
			Details:    details,
		}
	}

	// 1. nil details -> {}
	log, err := service.Record(context.Background(), baseInput(nil))
	if err != nil || string(log.Details) != "{}" {
		t.Fatalf("expected {} for nil details, got err=%v details=%s", err, string(log.Details))
	}

	// 2. valid json.RawMessage
	raw := json.RawMessage(`{"key":"value"}`)
	log, err = service.Record(context.Background(), baseInput(raw))
	if err != nil || string(log.Details) != `{"key":"value"}` {
		t.Fatalf("expected raw preserved, got err=%v details=%s", err, string(log.Details))
	}

	// 3. valid []byte
	log, err = service.Record(context.Background(), baseInput([]byte(`{"number":123}`)))
	if err != nil || string(log.Details) != `{"number":123}` {
		t.Fatalf("expected byte slice preserved, got err=%v details=%s", err, string(log.Details))
	}

	// 4. valid string
	log, err = service.Record(context.Background(), baseInput(`{"str":"hello"}`))
	if err != nil || string(log.Details) != `{"str":"hello"}` {
		t.Fatalf("expected string preserved, got err=%v details=%s", err, string(log.Details))
	}

	// 5. invalid string JSON
	_, err = service.Record(context.Background(), baseInput(`{not-json}`))
	if !errors.Is(err, ErrInvalidDetails) {
		t.Fatalf("expected ErrInvalidDetails for bad string JSON, got: %v", err)
	}

	// 6. invalid byte slice JSON
	_, err = service.Record(context.Background(), baseInput([]byte(`{not-json}`)))
	if !errors.Is(err, ErrInvalidDetails) {
		t.Fatalf("expected ErrInvalidDetails for bad bytes JSON, got: %v", err)
	}
}

func TestAuditService_Record_ValidationErrors(t *testing.T) {
	mockRepo := &mockActivityLogRepository{}
	service := NewService(mockRepo)

	// Missing EventType
	_, err := service.Record(context.Background(), RecordInput{
		Actor:      SystemActor(),
		TargetType: domain.TargetTypeCampaign,
		TargetID:   "cmp-1",
		Summary:    "Summary",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for missing event type, got: %v", err)
	}

	// Invalid Actor (UserActor with empty ID)
	_, err = service.Record(context.Background(), RecordInput{
		EventType:  domain.EventCampaignCreated,
		Actor:      UserActor("", nil, nil),
		TargetType: domain.TargetTypeCampaign,
		TargetID:   "cmp-1",
		Summary:    "Summary",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid actor, got: %v", err)
	}

	// Invalid TargetType
	_, err = service.Record(context.Background(), RecordInput{
		EventType:  domain.EventCampaignCreated,
		Actor:      SystemActor(),
		TargetType: "unsupported",
		TargetID:   "cmp-1",
		Summary:    "Summary",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid target type, got: %v", err)
	}

	// Missing TargetID
	_, err = service.Record(context.Background(), RecordInput{
		EventType:  domain.EventCampaignCreated,
		Actor:      SystemActor(),
		TargetType: domain.TargetTypeCampaign,
		TargetID:   "",
		Summary:    "Summary",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for missing target ID, got: %v", err)
	}

	// Missing Summary
	_, err = service.Record(context.Background(), RecordInput{
		EventType:  domain.EventCampaignCreated,
		Actor:      SystemActor(),
		TargetType: domain.TargetTypeCampaign,
		TargetID:   "cmp-1",
		Summary:    "",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for missing summary, got: %v", err)
	}

	// Invalid Status
	_, err = service.Record(context.Background(), RecordInput{
		EventType:  domain.EventCampaignCreated,
		Actor:      SystemActor(),
		TargetType: domain.TargetTypeCampaign,
		TargetID:   "cmp-1",
		Summary:    "Summary",
		Status:     "unknown",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid status, got: %v", err)
	}
}

func TestAuditService_Record_RepositoryErrorPropagation(t *testing.T) {
	dbErr := errors.New("database connection lost")
	mockRepo := &mockActivityLogRepository{
		createFunc: func(ctx context.Context, log *domain.ActivityLog) error {
			return dbErr
		},
	}

	service := NewService(mockRepo)
	_, err := service.Record(context.Background(), RecordInput{
		EventType:  domain.EventCampaignCreated,
		Actor:      SystemActor(),
		TargetType: domain.TargetTypeCampaign,
		TargetID:   "cmp-1",
		Summary:    "Created campaign",
	})

	if !errors.Is(err, dbErr) {
		t.Fatalf("expected repository error to be propagated, got: %v", err)
	}
}

func TestAuditService_GetByIDAndList(t *testing.T) {
	mockLog := &domain.ActivityLog{
		ID:        "act-999",
		EventType: domain.EventSettingsUpdated,
		ActorType: domain.ActorTypeSystem,
		Summary:   "Updated settings",
	}

	mockRepo := &mockActivityLogRepository{
		getByIDFunc: func(ctx context.Context, id string) (*domain.ActivityLog, error) {
			if id == "act-999" {
				return mockLog, nil
			}
			return nil, domain.ErrNotFound
		},
		listFunc: func(ctx context.Context, filter domain.ActivityLogFilter) ([]*domain.ActivityLog, int, error) {
			return []*domain.ActivityLog{mockLog}, 1, nil
		},
	}

	service := NewService(mockRepo)

	// 1. GetByID success
	found, err := service.GetByID(context.Background(), "act-999")
	if err != nil || found.ID != "act-999" {
		t.Fatalf("expected found log act-999, got err=%v, found=%+v", err, found)
	}

	// 2. GetByID empty ID
	_, err = service.GetByID(context.Background(), "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for empty ID, got: %v", err)
	}

	// 3. GetByID not found
	_, err = service.GetByID(context.Background(), "act-nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	// 4. List success
	items, total, err := service.List(context.Background(), domain.ActivityLogFilter{Limit: 10})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("expected 1 item from list, got err=%v, total=%d, len=%d", err, total, len(items))
	}
}
