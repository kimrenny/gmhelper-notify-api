-- Migration 0004: Automation Rule Redesign
-- Remove obsolete event_type trigger model and add schedule evaluation timestamps

-- Clear existing obsolete automation rules that used event-based triggers
DELETE FROM automation_rules;

-- Remove obsolete event_type column
ALTER TABLE automation_rules DROP COLUMN IF EXISTS event_type;

-- Add evaluation tracking columns
ALTER TABLE automation_rules ADD COLUMN IF NOT EXISTS last_evaluated_at timestamptz;
ALTER TABLE automation_rules ADD COLUMN IF NOT EXISTS next_evaluation_at timestamptz;

-- Index for finding enabled automation rules due for scheduled evaluation
CREATE INDEX IF NOT EXISTS idx_automation_rules_next_eval ON automation_rules (enabled, next_evaluation_at);
