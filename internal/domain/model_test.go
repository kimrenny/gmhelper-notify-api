package domain

import (
	"context"
	"testing"
	"time"
)

func TestEmailTemplateValidate(t *testing.T) {
	template := &EmailTemplate{
		ID:          "template-1",
		TemplateKey: "welcome_email",
		Name:        "Welcome",
		Subject:     "Welcome to GMHelper",
		HTMLBody:    "<p>Hello</p>",
		Locale:      "en-US",
		Status:      TemplateStatusActive,
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := template.Validate(context.Background()); err != nil {
		t.Fatalf("expected valid template, got %v", err)
	}
}

func TestEmailTemplateValidateInvalid(t *testing.T) {
	template := &EmailTemplate{}
	if err := template.Validate(context.Background()); err == nil {
		t.Fatal("expected invalid template error")
	}
}

func TestNotificationCampaignValidate(t *testing.T) {
	campaign := &NotificationCampaign{
		ID:           "campaign-1",
		Name:         "Launch Campaign",
		TemplateID:   "template-1",
		CampaignType: "email",
		Status:       CampaignStatusScheduled,
		ScheduledAt:  time.Now().UTC(),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := campaign.Validate(context.Background()); err != nil {
		t.Fatalf("expected valid campaign, got %v", err)
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
		EventType:  "user_signed_up",
		TemplateID: "template-1",
		Enabled:    true,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := rule.Validate(context.Background()); err != nil {
		t.Fatalf("expected valid rule, got %v", err)
	}
}
