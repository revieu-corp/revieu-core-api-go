-- +goose Up

CREATE TABLE IF NOT EXISTS dishes (
    id BIGSERIAL PRIMARY KEY,
    merchant_id BIGINT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    store_id BIGINT REFERENCES stores(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    image_url VARCHAR(512),
    description TEXT,
    original_price NUMERIC(10, 2) DEFAULT 0,
    category VARCHAR(100),
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Existing dev databases may already contain an older dishes table without
-- the store relationship. Add the column incrementally before creating its
-- index so this migration is safe for both fresh and historical schemas.
ALTER TABLE dishes
    ADD COLUMN IF NOT EXISTS store_id BIGINT REFERENCES stores(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_dishes_merchant ON dishes(merchant_id);
CREATE INDEX IF NOT EXISTS idx_dishes_store ON dishes(store_id);
CREATE INDEX IF NOT EXISTS idx_dishes_deleted_at ON dishes(deleted_at);

ALTER TABLE coupons
    ADD COLUMN IF NOT EXISTS dish_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down

ALTER TABLE coupons DROP COLUMN IF EXISTS dish_ids;
DROP INDEX IF EXISTS idx_dishes_deleted_at;
DROP INDEX IF EXISTS idx_dishes_store;
DROP INDEX IF EXISTS idx_dishes_merchant;
DROP TABLE IF EXISTS dishes;
