-- +goose Up

ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS dedup_key VARCHAR(255);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_dedup_key
    ON notifications (dedup_key)
    WHERE dedup_key IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_notifications_dedup_key;
ALTER TABLE notifications DROP COLUMN IF EXISTS dedup_key;
