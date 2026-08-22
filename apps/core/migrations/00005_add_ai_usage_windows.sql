-- +goose Up

CREATE TABLE IF NOT EXISTS ai_usage_windows (
    scope VARCHAR(32) NOT NULL,
    scope_key VARCHAR(128) NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (scope, scope_key, window_start)
);

CREATE INDEX IF NOT EXISTS idx_ai_usage_windows_window_start
    ON ai_usage_windows (window_start);

-- +goose Down

DROP TABLE IF EXISTS ai_usage_windows;
