-- +goose Up

CREATE TABLE IF NOT EXISTS payment_attempts (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL,
    amount NUMERIC(10, 2) NOT NULL,
    currency VARCHAR(10) NOT NULL,
    payment_method VARCHAR(30),
    provider_reference VARCHAR(255),
    payment_id BIGINT REFERENCES payments(id) ON DELETE SET NULL,
    failure_reason TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payment_attempt_order_key UNIQUE (order_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_payment_attempts_status_started
    ON payment_attempts(status, started_at);
CREATE INDEX IF NOT EXISTS idx_payment_attempts_user_created
    ON payment_attempts(user_id, created_at DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_payment_attempts_user_created;
DROP INDEX IF EXISTS idx_payment_attempts_status_started;
DROP TABLE IF EXISTS payment_attempts;
