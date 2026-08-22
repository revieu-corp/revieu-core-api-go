-- +goose Up

CREATE TABLE IF NOT EXISTS operational_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id BIGINT NOT NULL,
    actor_role VARCHAR(30) NOT NULL,
    action VARCHAR(80) NOT NULL,
    target_type VARCHAR(30) NOT NULL,
    target_id BIGINT,
    result VARCHAR(20) NOT NULL,
    error_class VARCHAR(30),
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_operational_audit_action_created
    ON operational_audit_logs(action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_operational_audit_actor_created
    ON operational_audit_logs(actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_operational_audit_result_created
    ON operational_audit_logs(result, created_at DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_operational_audit_result_created;
DROP INDEX IF EXISTS idx_operational_audit_actor_created;
DROP INDEX IF EXISTS idx_operational_audit_action_created;
DROP TABLE IF EXISTS operational_audit_logs;
