-- Migration 0007: Create Automation Executions
-- Tracks rule execution history for persistent idempotency and cooldown enforcement.

CREATE TABLE automation_executions (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    rule_id uuid NOT NULL,
    event_id text NOT NULL,
    recipient_email text NOT NULL,
    external_user_id text,
    notification_id uuid,
    status text NOT NULL,
    executed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_automation_rule_event UNIQUE (rule_id, event_id),
    FOREIGN KEY (rule_id) REFERENCES automation_rules(id) ON DELETE CASCADE
);

-- Index for cooldown lookup by rule and recipient email
CREATE INDEX idx_automation_exec_cooldown ON automation_executions (rule_id, recipient_email, executed_at DESC);

-- Index for cooldown lookup by rule and external user id
CREATE INDEX idx_automation_exec_user_cooldown ON automation_executions (rule_id, external_user_id, executed_at DESC);
