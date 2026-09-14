-- Add unique index for campaign recipients on (campaign_id, external_user_id) for idempotent audience population

CREATE UNIQUE INDEX IF NOT EXISTS idx_campaign_recipients_campaign_user ON campaign_recipients (campaign_id, external_user_id);
