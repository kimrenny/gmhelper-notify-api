-- Allow scheduled_at to be NULL for unscheduled or draft campaigns
ALTER TABLE notification_campaigns ALTER COLUMN scheduled_at DROP NOT NULL;
