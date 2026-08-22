-- +goose Up

-- The QB merchant flow persists dishes and associates coupons with one or
-- more merchant-owned dishes. Keep this migration separate from the initial
-- schema so a normal deployment (DB_AUTO_MIGRATE=false) has an explicit,
-- reviewable upgrade path.
CREATE TABLE IF NOT EXISTS dishes (
    id BIGSERIAL PRIMARY KEY,
    merchant_id BIGINT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    image_url VARCHAR(255),
    description TEXT,
    original_price NUMERIC(10,2) NOT NULL DEFAULT 0,
    category VARCHAR(50),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_dishes_merchant_id ON dishes(merchant_id);
CREATE INDEX IF NOT EXISTS idx_dishes_deleted_at ON dishes(deleted_at);

ALTER TABLE coupons
    ADD COLUMN IF NOT EXISTS dish_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down

ALTER TABLE coupons DROP COLUMN IF EXISTS dish_ids;
DROP INDEX IF EXISTS idx_dishes_deleted_at;
DROP INDEX IF EXISTS idx_dishes_merchant_id;
DROP TABLE IF EXISTS dishes;
