-- Migration 0005: Add Template Type to Email Templates
-- Supports explicit notification scenario classification: direct, campaign, user_agreement, automation

-- 1. Add nullable template_type column
ALTER TABLE email_templates ADD COLUMN IF NOT EXISTS template_type text;

-- 2. Backfill existing records with deterministic default 'campaign'
UPDATE email_templates SET template_type = 'campaign' WHERE template_type IS NULL;

-- 3. Enforce NOT NULL constraint
ALTER TABLE email_templates ALTER COLUMN template_type SET NOT NULL;

-- 4. Add check constraint for valid template types
ALTER TABLE email_templates DROP CONSTRAINT IF EXISTS chk_email_templates_type;
ALTER TABLE email_templates ADD CONSTRAINT chk_email_templates_type CHECK (template_type IN ('direct', 'campaign', 'user_agreement', 'automation'));

-- 5. Add index on template_type
CREATE INDEX IF NOT EXISTS idx_email_templates_type ON email_templates (template_type);
