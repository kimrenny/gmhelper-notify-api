package domain

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestActivityEnums_Validation(t *testing.T) {
	// ActorType
	if !ActorTypeUser.IsValid() || !ActorTypeSystem.IsValid() || !ActorTypeService.IsValid() {
		t.Fatal("expected standard actor types to be valid")
	}
	if ActivityActorType("invalid").IsValid() {
		t.Fatal("expected invalid actor type to fail validation")
	}

	// Status
	if !ActivityStatusSuccess.IsValid() || !ActivityStatusFailure.IsValid() || !ActivityStatusWarning.IsValid() {
		t.Fatal("expected standard status values to be valid")
	}
	if ActivityStatus("unknown").IsValid() {
		t.Fatal("expected unknown status to fail validation")
	}

	// TargetType
	validTargets := []ActivityTargetType{
		TargetTypeCampaign,
		TargetTypeDirectNotification,
		TargetTypeTemplate,
		TargetTypeAutomationRule,
		TargetTypeSettings,
		TargetTypeAgreement,
	}
	for _, target := range validTargets {
		if !target.IsValid() {
			t.Fatalf("expected target type %s to be valid", target)
		}
	}
	if ActivityTargetType("unsupported").IsValid() {
		t.Fatal("expected unsupported target type to fail validation")
	}
}

func TestActivityLog_Validate(t *testing.T) {
	ctx := context.Background()
	actorID := "usr-123"
	actorName := "Admin User"
	actorRole := "Owner"
	targetName := "Welcome Template"

	validLog := &ActivityLog{
		ID:          "act-123",
		EventType:   "template.created",
		ActorType:   ActorTypeUser,
		ActorUserID: &actorID,
		ActorName:   &actorName,
		ActorRole:   &actorRole,
		TargetType:  TargetTypeTemplate,
		TargetID:    "tmpl-456",
		TargetName:  &targetName,
		Status:      ActivityStatusSuccess,
		Summary:     "Created welcome template",
		Details:     json.RawMessage(`{"version":1}`),
		CreatedAt:   time.Now().UTC(),
	}

	if err := validLog.Validate(ctx); err != nil {
		t.Fatalf("expected valid log to pass validation, got: %v", err)
	}

	// System actor with null actor fields
	systemLog := &ActivityLog{
		ID:         "act-124",
		EventType:  "campaign.completed",
		ActorType:  ActorTypeSystem,
		TargetType: TargetTypeCampaign,
		TargetID:   "cmp-789",
		Status:     ActivityStatusSuccess,
		Summary:    "Campaign completed delivery",
		CreatedAt:  time.Now().UTC(),
	}
	if err := systemLog.Validate(ctx); err != nil {
		t.Fatalf("expected valid system log to pass validation, got: %v", err)
	}

	// Missing ID
	invalidLog := *validLog
	invalidLog.ID = ""
	if err := invalidLog.Validate(ctx); err != ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for missing ID, got: %v", err)
	}

	// Missing EventType
	invalidLog = *validLog
	invalidLog.EventType = ""
	if err := invalidLog.Validate(ctx); err != ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for missing EventType, got: %v", err)
	}

	// Missing TargetID
	invalidLog = *validLog
	invalidLog.TargetID = ""
	if err := invalidLog.Validate(ctx); err != ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for missing TargetID, got: %v", err)
	}

	// Missing Summary
	invalidLog = *validLog
	invalidLog.Summary = ""
	if err := invalidLog.Validate(ctx); err != ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for missing Summary, got: %v", err)
	}

	// Invalid ActorType
	invalidLog = *validLog
	invalidLog.ActorType = "invalid"
	if err := invalidLog.Validate(ctx); err != ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for invalid ActorType, got: %v", err)
	}

	// Invalid TargetType
	invalidLog = *validLog
	invalidLog.TargetType = "invalid"
	if err := invalidLog.Validate(ctx); err != ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for invalid TargetType, got: %v", err)
	}

	// Invalid Status
	invalidLog = *validLog
	invalidLog.Status = "invalid"
	if err := invalidLog.Validate(ctx); err != ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for invalid Status, got: %v", err)
	}

	// Invalid Details JSON
	invalidLog = *validLog
	invalidLog.Details = json.RawMessage(`{not-json}`)
	if err := invalidLog.Validate(ctx); err != ErrInvalidEntity {
		t.Fatalf("expected ErrInvalidEntity for invalid JSON details, got: %v", err)
	}
}
