package domain

import (
	"context"
	"testing"
	"time"
)

func TestTemplateTypeIsValid(t *testing.T) {
	validTypes := []TemplateType{
		TemplateTypeDirect,
		TemplateTypeCampaign,
		TemplateTypeUserAgreement,
		TemplateTypeAutomation,
	}

	for _, typ := range validTypes {
		if !typ.IsValid() {
			t.Errorf("expected template type %q to be valid", typ)
		}
	}

	invalidTypes := []TemplateType{
		"",
		"unknown",
		"invalid",
		"marketing",
		"notification",
	}

	for _, typ := range invalidTypes {
		if typ.IsValid() {
			t.Errorf("expected template type %q to be invalid", typ)
		}
	}
}

func TestEmailTemplateValidate(t *testing.T) {
	validTypes := []TemplateType{
		TemplateTypeDirect,
		TemplateTypeCampaign,
		TemplateTypeUserAgreement,
		TemplateTypeAutomation,
	}

	for _, typ := range validTypes {
		template := &EmailTemplate{
			ID:           "template-1",
			TemplateKey:  "welcome_email",
			Name:         "Welcome",
			TemplateType: typ,
			Subject:      "Welcome to GMHelper",
			HTMLBody:     "<p>Hello</p>",
			Locale:       "en-US",
			Status:       TemplateStatusActive,
			Version:      1,
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		}
		if err := template.Validate(context.Background()); err != nil {
			t.Fatalf("expected valid template with type %s, got %v", typ, err)
		}
	}
}

func TestEmailTemplateValidateInvalid(t *testing.T) {
	template := &EmailTemplate{}
	if err := template.Validate(context.Background()); err == nil {
		t.Fatal("expected invalid template error")
	}

	// Template with missing/invalid type
	invalidTypeTemplate := &EmailTemplate{
		ID:           "template-1",
		TemplateKey:  "welcome_email",
		Name:         "Welcome",
		TemplateType: "invalid_type",
		Subject:      "Welcome to GMHelper",
		HTMLBody:     "<p>Hello</p>",
		Locale:       "en-US",
		Status:       TemplateStatusActive,
		Version:      1,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := invalidTypeTemplate.Validate(context.Background()); err == nil {
		t.Fatal("expected invalid template error for invalid template type")
	}
}

func TestCampaignStatusIsValid(t *testing.T) {
	validStatuses := []CampaignStatus{
		CampaignStatusDraft,
		CampaignStatusScheduled,
		CampaignStatusRunning,
		CampaignStatusSending,
		CampaignStatusCompleted,
		CampaignStatusPartiallyFailed,
		CampaignStatusFailed,
		CampaignStatusCancelled,
	}

	for _, status := range validStatuses {
		if !status.IsValid() {
			t.Errorf("expected status %q to be valid", status)
		}
	}

	invalidStatuses := []CampaignStatus{
		"",
		"unknown",
		"invalid",
		"paused",
	}

	for _, status := range invalidStatuses {
		if status.IsValid() {
			t.Errorf("expected status %q to be invalid", status)
		}
	}
}

func TestNotificationCampaignValidate(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name     string
		campaign NotificationCampaign
		wantErr  bool
	}{
		{
			name: "valid scheduled campaign",
			campaign: NotificationCampaign{
				ID:           "campaign-1",
				Name:         "Launch Campaign",
				TemplateID:   "template-1",
				CampaignType: "email",
				Status:       CampaignStatusScheduled,
				ScheduledAt:  &now,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			wantErr: false,
		},
		{
			name: "valid running campaign without scheduledAt",
			campaign: NotificationCampaign{
				ID:           "campaign-2",
				Name:         "Immediate Campaign",
				TemplateID:   "template-1",
				CampaignType: "email",
				Status:       CampaignStatusRunning,
				ScheduledAt:  nil,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			wantErr: false,
		},
		{
			name: "valid draft campaign with nil scheduledAt",
			campaign: NotificationCampaign{
				ID:           "campaign-3",
				Name:         "Draft Campaign",
				TemplateID:   "template-1",
				CampaignType: "broadcast",
				Status:       CampaignStatusDraft,
				ScheduledAt:  nil,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			wantErr: false,
		},
		{
			name: "invalid status",
			campaign: NotificationCampaign{
				ID:           "campaign-4",
				Name:         "Invalid Status Campaign",
				TemplateID:   "template-1",
				CampaignType: "broadcast",
				Status:       CampaignStatus("bogus"),
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			campaign: NotificationCampaign{
				ID:           "campaign-5",
				Name:         "",
				TemplateID:   "template-1",
				CampaignType: "broadcast",
				Status:       CampaignStatusDraft,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			wantErr: true,
		},
		{
			name: "missing template ID",
			campaign: NotificationCampaign{
				ID:           "campaign-6",
				Name:         "Campaign Name",
				TemplateID:   "",
				CampaignType: "broadcast",
				Status:       CampaignStatusDraft,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.campaign.Validate(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCampaignRecipientValidate(t *testing.T) {
	recipient := &CampaignRecipient{
		ID:             "recipient-1",
		CampaignID:     "campaign-1",
		RecipientEmail: "user@example.com",
		DeliveryStatus: DeliveryStatusPending,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := recipient.Validate(context.Background()); err != nil {
		t.Fatalf("expected valid recipient, got %v", err)
	}
}

func TestDirectNotificationValidate(t *testing.T) {
	notification := &DirectNotification{
		ID:               "direct-1",
		TemplateID:       "template-1",
		NotificationType: NotificationTypeDirect,
		RecipientEmail:   "user@example.com",
		DeliveryStatus:   DeliveryStatusPending,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	if err := notification.Validate(context.Background()); err != nil {
		t.Fatalf("expected valid direct notification, got %v", err)
	}
}

func TestAutomationRuleValidate(t *testing.T) {
	rule := &AutomationRule{
		ID:         "rule-1",
		Name:       "Notify on signup",
		TemplateID: "template-1",
		Enabled:    true,
		Config: AutomationRuleConfig{
			Version: 1,
			Schedule: ScheduleConfig{
				Type:      ScheduleTypeDaily,
				HourUTC:   intPtr(3),
				MinuteUTC: intPtr(0),
			},
			Conditions: ConditionGroup{
				Operator: GroupOperatorAll,
				Conditions: []ConditionNode{
					{
						Item: &ConditionItem{
							Field:    FieldIsActive,
							Operator: OperatorEquals,
							Value:    true,
						},
					},
				},
			},
			Action: ActionConfig{
				Type: ActionTypeSendEmail,
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := rule.Validate(context.Background()); err != nil {
		t.Fatalf("expected valid rule, got %v", err)
	}

	invalidRule := &AutomationRule{
		ID:         "rule-1",
		Name:       "Missing config",
		TemplateID: "template-1",
	}
	if err := invalidRule.Validate(context.Background()); err == nil {
		t.Fatalf("expected error on invalid rule, got nil")
	}
}

func TestDeliveryAttemptValidate(t *testing.T) {
	attempt := &DeliveryAttempt{
		ID:            "attempt-1",
		TargetType:    DeliveryTargetDirectNotification,
		TargetID:      "direct-1",
		Status:        DeliveryStatusPending,
		AttemptNumber: 1,
		AttemptedAt:   time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}
	if err := attempt.Validate(context.Background()); err != nil {
		t.Fatalf("expected valid delivery attempt, got %v", err)
	}
}

func TestDeliveryAttemptValidateInvalid(t *testing.T) {
	attempt := &DeliveryAttempt{
		ID: "attempt-1",
	}
	if err := attempt.Validate(context.Background()); err == nil {
		t.Fatal("expected invalid delivery attempt error, got nil")
	}
}
