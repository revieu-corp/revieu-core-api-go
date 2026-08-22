-- +goose Up
ALTER TABLE reviews
    ADD COLUMN IF NOT EXISTS location_verified BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE reviews
    DROP COLUMN IF EXISTS location_verified;
