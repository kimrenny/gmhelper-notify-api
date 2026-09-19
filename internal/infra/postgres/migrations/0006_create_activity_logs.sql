-- Migration 0006: Create Activity Logs
-- Provides an immutable, append-only audit trail for administrative and business operations.

CREATE TABLE activity_logs (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type text NOT NULL,
    actor_type text NOT NULL,
    actor_user_id text,
    actor_name text,
    actor_role text,
    target_type text NOT NULL,
    target_id text NOT NULL,
    target_name text,
    status text NOT NULL,
    summary text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    error_message text,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Indexes for efficient Owner-only filtering and chronological pagination
CREATE INDEX idx_activity_logs_created_at ON activity_logs (created_at DESC, id DESC);
CREATE INDEX idx_activity_logs_event_type ON activity_logs (event_type);
CREATE INDEX idx_activity_logs_target ON activity_logs (target_type, target_id);
CREATE INDEX idx_activity_logs_actor_user_id ON activity_logs (actor_user_id);
CREATE INDEX idx_activity_logs_status ON activity_logs (status);
