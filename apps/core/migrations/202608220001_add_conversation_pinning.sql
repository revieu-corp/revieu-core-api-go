-- +goose Up
ALTER TABLE conversation_participants
    ADD COLUMN IF NOT EXISTS is_pinned BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE conversation_participants
    DROP COLUMN IF EXISTS is_pinned;
