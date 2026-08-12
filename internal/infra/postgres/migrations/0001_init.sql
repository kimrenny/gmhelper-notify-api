-- Initialize PostgreSQL schema for GMHelper Notify API

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE email_templates (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_key text NOT NULL,
    name text NOT NULL,
    subject text NOT NULL,
    html_body text NOT NULL,
    plain_text_body text,
    locale text NOT NULL,
    status text NOT NULL,
    version integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (template_key, locale, version)
);

CREATE INDEX idx_email_templates_template_key ON email_templates (template_key);
CREATE INDEX idx_email_templates_locale ON email_templates (locale);
CREATE INDEX idx_email_templates_status ON email_templates (status);

CREATE TABLE notification_campaigns (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    name text NOT NULL,
    template_id uuid NOT NULL,
    campaign_type text NOT NULL,
    status text NOT NULL,
    scheduled_at timestamptz NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (template_id) REFERENCES email_templates(id) ON DELETE RESTRICT
);

CREATE INDEX idx_notification_campaigns_status ON notification_campaigns (status);
CREATE INDEX idx_notification_campaigns_scheduled_at ON notification_campaigns (scheduled_at);

CREATE TABLE campaign_recipients (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id uuid NOT NULL,
    external_user_id text,
    recipient_email text NOT NULL,
    recipient_name text,
    delivery_status text NOT NULL,
    attempts_count integer NOT NULL DEFAULT 0,
    last_attempt_at timestamptz,
    sent_at timestamptz,
    error_message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (campaign_id) REFERENCES notification_campaigns(id) ON DELETE CASCADE
);

CREATE INDEX idx_campaign_recipients_campaign_id ON campaign_recipients (campaign_id);
CREATE INDEX idx_campaign_recipients_delivery_status ON campaign_recipients (delivery_status);

CREATE TABLE direct_notifications (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id uuid NOT NULL,
    external_user_id text,
    recipient_email text NOT NULL,
    recipient_name text,
    notification_type text NOT NULL,
    delivery_status text NOT NULL,
    attempts_count integer NOT NULL DEFAULT 0,
    last_attempt_at timestamptz,
    sent_at timestamptz,
    error_message text,
    payload jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (template_id) REFERENCES email_templates(id) ON DELETE RESTRICT
);

CREATE INDEX idx_direct_notifications_delivery_status ON direct_notifications (delivery_status);
CREATE INDEX idx_direct_notifications_notification_type ON direct_notifications (notification_type);

CREATE TABLE delivery_attempts (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    target_type text NOT NULL,
    target_id uuid NOT NULL,
    status text NOT NULL,
    attempt_number integer NOT NULL,
    error_message text,
    attempted_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_delivery_attempts_target ON delivery_attempts (target_type, target_id);
CREATE INDEX idx_delivery_attempts_status ON delivery_attempts (status);

CREATE TABLE automation_rules (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    name text NOT NULL,
    event_type text NOT NULL,
    template_id uuid NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (template_id) REFERENCES email_templates(id) ON DELETE RESTRICT
);

CREATE INDEX idx_automation_rules_enabled ON automation_rules (enabled);

CREATE TABLE app_settings (
    key text PRIMARY KEY,
    value text NOT NULL,
    category text NOT NULL,
    description text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_app_settings_category ON app_settings (category);
