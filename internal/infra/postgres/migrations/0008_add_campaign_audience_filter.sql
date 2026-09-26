ALTER TABLE notification_campaigns ADD COLUMN IF NOT EXISTS audience_filter JSONB;
