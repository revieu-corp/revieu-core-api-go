-- +goose Up

-- =============================================
-- RevieU Database Initialization Script (v2)
-- Platform: PostgreSQL 14+
-- =============================================

-- =============================================
-- A. User System
-- =============================================

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    status SMALLINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_auths (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    identity_type VARCHAR(20) NOT NULL,
    identifier VARCHAR(255) NOT NULL,
    credential VARCHAR(255),
    last_login_at TIMESTAMPTZ,
    UNIQUE (identity_type, identifier)
);

CREATE TABLE user_profiles (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    nickname VARCHAR(50) NOT NULL,
    avatar_url VARCHAR(255),
    intro VARCHAR(255),
    location VARCHAR(100),
    follower_count INT DEFAULT 0,
    following_count INT DEFAULT 0,
    post_count INT DEFAULT 0,
    review_count INT DEFAULT 0,
    like_count INT DEFAULT 0,
    coupon_count INT DEFAULT 0
);

CREATE TABLE email_verifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    token VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- B. Social System
-- =============================================

CREATE TABLE user_follows (
    id BIGSERIAL PRIMARY KEY,
    follower_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    following_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (follower_id, following_id)
);

CREATE TABLE merchant_follows (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    merchant_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, merchant_id)
);

CREATE TABLE likes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    target_type VARCHAR(20) NOT NULL,
    target_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, target_type, target_id)
);

CREATE TABLE favorites (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    target_type VARCHAR(20) NOT NULL,
    target_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, target_type, target_id)
);

-- =============================================
-- C. User Settings
-- =============================================

CREATE TABLE user_addresses (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    phone VARCHAR(20) NOT NULL,
    province VARCHAR(50),
    city VARCHAR(100),
    district VARCHAR(50),
    address VARCHAR(255) NOT NULL,
    postal_code VARCHAR(20),
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_privacies (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    is_public BOOLEAN DEFAULT true
);

CREATE TABLE user_notifications (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    push_enabled BOOLEAN DEFAULT true,
    email_enabled BOOLEAN DEFAULT true
);

CREATE TABLE account_deletions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    reason VARCHAR(255),
    scheduled_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- D. Merchant & Store System
-- =============================================

CREATE TABLE merchants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    name VARCHAR(100) NOT NULL,
    business_name VARCHAR(100),
    business_type VARCHAR(50),
    category VARCHAR(50),
    logo_url VARCHAR(255),
    address VARCHAR(255),
    phone VARCHAR(20),
    contact_phone VARCHAR(20),
    contact_email VARCHAR(255),
    website_url VARCHAR(255),
    social_links JSONB DEFAULT '{}',
    cover_image VARCHAR(255),
    description TEXT,
    avg_rating DECIMAL(3, 2) DEFAULT 0.00,
    review_count INT DEFAULT 0,
    follower_count INT DEFAULT 0,
    total_stores INT DEFAULT 0,
    total_reviews INT DEFAULT 0,
    verification_status VARCHAR(20) DEFAULT 'unverified',
    verified_at TIMESTAMPTZ,
    status SMALLINT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE categories (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    parent_id BIGINT REFERENCES categories(id) ON DELETE SET NULL,
    icon_url VARCHAR(255)
);

CREATE TABLE stores (
    id BIGSERIAL PRIMARY KEY,
    merchant_id BIGINT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    address VARCHAR(255),
    city VARCHAR(100),
    state VARCHAR(100),
    zip_code VARCHAR(20),
    country VARCHAR(50),
    phone VARCHAR(50),
    website VARCHAR(255),
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    cover_image_url VARCHAR(255),
    images JSONB DEFAULT '[]',
    avg_rating DECIMAL(3, 2) DEFAULT 0.00,
    review_count INT DEFAULT 0,
    follower_count INT DEFAULT 0,
    status SMALLINT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE merchant_follows
    ADD CONSTRAINT fk_merchant_follows_merchant
    FOREIGN KEY (merchant_id) REFERENCES merchants(id) ON DELETE CASCADE;

CREATE TABLE store_categories (
    store_id BIGINT REFERENCES stores(id) ON DELETE CASCADE,
    category_id BIGINT REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (store_id, category_id)
);

CREATE TABLE store_hours (
    id BIGSERIAL PRIMARY KEY,
    store_id BIGINT NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    day_of_week SMALLINT NOT NULL,
    open_time VARCHAR(10),
    close_time VARCHAR(10),
    is_closed BOOLEAN DEFAULT false
);

CREATE TABLE dishes (
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

CREATE INDEX idx_dishes_merchant ON dishes(merchant_id);
CREATE INDEX idx_dishes_store ON dishes(store_id);
CREATE INDEX idx_dishes_deleted_at ON dishes(deleted_at);

-- =============================================
-- E. Tags
-- =============================================

CREATE TABLE tags (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    type VARCHAR(20) DEFAULT 'general',
    post_count INT DEFAULT 0,
    review_count INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- F. Reviews & Content
-- =============================================

CREATE TABLE reviews (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    venue_id BIGINT NOT NULL,
    merchant_id BIGINT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    store_id BIGINT REFERENCES stores(id) ON DELETE SET NULL,
    rating REAL NOT NULL,
    rating_env REAL,
    rating_service REAL,
    rating_value REAL,
    rating_food REAL,
    content TEXT,
    images JSONB DEFAULT '[]',
    visit_date DATE NOT NULL,
    avg_cost INT,
    like_count INT DEFAULT 0,
    comment_count INT DEFAULT 0,
    status SMALLINT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE review_media (
    id BIGSERIAL PRIMARY KEY,
    review_id BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    media_type VARCHAR(10) NOT NULL CHECK (media_type IN ('image', 'video')),
    url VARCHAR(512) NOT NULL,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE review_comments (
    id BIGSERIAL PRIMARY KEY,
    review_id BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_comment_id BIGINT REFERENCES review_comments(id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    is_merchant_reply BOOLEAN DEFAULT false,
    status SMALLINT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE review_votes (
    id BIGSERIAL PRIMARY KEY,
    review_id BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    vote_type VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (review_id, user_id, vote_type)
);

CREATE TABLE review_tags (
    review_id BIGINT REFERENCES reviews(id) ON DELETE CASCADE,
    tag_id BIGINT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (review_id, tag_id)
);

CREATE TABLE posts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    merchant_id BIGINT REFERENCES merchants(id) ON DELETE SET NULL,
    review_id BIGINT REFERENCES reviews(id) ON DELETE SET NULL,
    post_type VARCHAR(20) DEFAULT 'general',
    title VARCHAR(100),
    content TEXT NOT NULL,
    images JSONB DEFAULT '[]',
    like_count INT DEFAULT 0,
    comment_count INT DEFAULT 0,
    share_count INT DEFAULT 0,
    view_count INT DEFAULT 0,
    is_pinned BOOLEAN DEFAULT false,
    status SMALLINT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE post_tags (
    post_id BIGINT REFERENCES posts(id) ON DELETE CASCADE,
    tag_id BIGINT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, tag_id)
);

CREATE TABLE post_comments (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_comment_id BIGINT REFERENCES post_comments(id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    like_count INT DEFAULT 0,
    status SMALLINT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- G. Commerce (Packages, Coupons, Orders, Vouchers, Payments)
-- =============================================

CREATE TABLE packages (
    id BIGSERIAL PRIMARY KEY,
    merchant_id BIGINT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    store_id BIGINT REFERENCES stores(id) ON DELETE SET NULL,
    title VARCHAR(100) NOT NULL,
    description TEXT,
    image_url VARCHAR(255),
    original_price NUMERIC(10, 2),
    sale_price NUMERIC(10, 2),
    discount_percentage NUMERIC(5, 2),
    total_quantity INT DEFAULT 0,
    sold_count INT DEFAULT 0,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    terms TEXT,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE coupons (
    id BIGSERIAL PRIMARY KEY,
    merchant_id BIGINT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    store_id BIGINT REFERENCES stores(id) ON DELETE SET NULL,
    package_id BIGINT REFERENCES packages(id) ON DELETE SET NULL,
    title VARCHAR(100) NOT NULL,
    description TEXT,
    image_url VARCHAR(255),
    type VARCHAR(20) NOT NULL,
    coupon_type VARCHAR(20),
    value VARCHAR(50),
    price NUMERIC(10, 2) DEFAULT 0,
    original_price NUMERIC(10, 2),
    sale_price NUMERIC(10, 2),
    discount_percentage NUMERIC(5, 2),
    dish_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    total_quantity INT DEFAULT 0,
    claimed_count INT DEFAULT 0,
    redeemed_count INT DEFAULT 0,
    max_per_user INT DEFAULT 1,
    terms TEXT,
    expiry_date TIMESTAMPTZ,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    coupon_id BIGINT REFERENCES coupons(id) ON DELETE SET NULL,
    package_id BIGINT REFERENCES packages(id) ON DELETE SET NULL,
    merchant_id BIGINT REFERENCES merchants(id) ON DELETE SET NULL,
    store_id BIGINT REFERENCES stores(id) ON DELETE SET NULL,
    quantity INT DEFAULT 1,
    total_price NUMERIC(10, 2),
    status VARCHAR(20) DEFAULT 'pending',
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE vouchers (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(50) NOT NULL UNIQUE,
    scan_token VARCHAR(128) UNIQUE,
    coupon_id BIGINT NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    package_id BIGINT REFERENCES packages(id) ON DELETE SET NULL,
    order_id BIGINT REFERENCES orders(id) ON DELETE SET NULL,
    merchant_id BIGINT REFERENCES merchants(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    expiry_date TIMESTAMPTZ,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    redeemed_at TIMESTAMPTZ,
    redeemed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    redemption_note TEXT,
    qr_code VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    amount NUMERIC(10, 2) NOT NULL,
    currency VARCHAR(10) NOT NULL,
    status VARCHAR(20) NOT NULL,
    coupon_id BIGINT REFERENCES coupons(id) ON DELETE SET NULL,
    merchant_id BIGINT REFERENCES merchants(id) ON DELETE SET NULL,
    order_id BIGINT REFERENCES orders(id) ON DELETE SET NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    payment_method VARCHAR(30),
    payment_session_id VARCHAR(255),
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- H. Media
-- =============================================

CREATE TABLE media_uploads (
    id BIGSERIAL PRIMARY KEY,
    uuid VARCHAR(36) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    object_key VARCHAR(512) NOT NULL,
    file_url VARCHAR(512),
    file_size BIGINT,
    mime_type VARCHAR(100),
    r2_bucket VARCHAR(100),
    status VARCHAR(20) DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- I. Messaging
-- =============================================

CREATE TABLE conversations (
    id BIGSERIAL PRIMARY KEY,
    type VARCHAR(20) DEFAULT 'direct',
    title VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE conversation_participants (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) DEFAULT 'member',
    is_muted BOOLEAN DEFAULT false,
    last_read_at TIMESTAMPTZ,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    message_type VARCHAR(20) DEFAULT 'text',
    attachments JSONB DEFAULT '[]',
    is_read BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- J. Merchant Verification
-- =============================================

CREATE TABLE merchant_verifications (
    id BIGSERIAL PRIMARY KEY,
    merchant_id BIGINT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    document_type VARCHAR(50) NOT NULL,
    document_url VARCHAR(512) NOT NULL,
    business_license VARCHAR(255),
    status VARCHAR(20) DEFAULT 'pending',
    reviewed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    rejection_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- K. Marketing
-- =============================================

CREATE TABLE marketing_posts (
    id BIGSERIAL PRIMARY KEY,
    merchant_id BIGINT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    store_id BIGINT REFERENCES stores(id) ON DELETE SET NULL,
    title VARCHAR(100) NOT NULL,
    content TEXT,
    images JSONB DEFAULT '[]',
    post_type VARCHAR(20) DEFAULT 'promotion',
    coupon_id BIGINT REFERENCES coupons(id) ON DELETE SET NULL,
    start_date TIMESTAMPTZ,
    end_date TIMESTAMPTZ,
    view_count INT DEFAULT 0,
    click_count INT DEFAULT 0,
    status VARCHAR(20) DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- L. Analytics
-- =============================================

CREATE TABLE merchant_analytics (
    id BIGSERIAL PRIMARY KEY,
    merchant_id BIGINT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    store_id BIGINT REFERENCES stores(id) ON DELETE SET NULL,
    date DATE NOT NULL,
    total_views INT DEFAULT 0,
    unique_visitors INT DEFAULT 0,
    reviews_received INT DEFAULT 0,
    coupons_redeemed INT DEFAULT 0,
    revenue NUMERIC(12, 2) DEFAULT 0,
    avg_rating DECIMAL(3, 2) DEFAULT 0.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- M. Notifications
-- =============================================

CREATE TABLE notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(255),
    content TEXT,
    data JSONB DEFAULT '{}',
    is_read BOOLEAN DEFAULT false,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- N. Reports & Admin
-- =============================================

CREATE TABLE reports (
    id BIGSERIAL PRIMARY KEY,
    reporter_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_type VARCHAR(20) NOT NULL,
    target_id BIGINT NOT NULL,
    reason VARCHAR(50) NOT NULL,
    description TEXT,
    status VARCHAR(20) DEFAULT 'pending',
    reviewed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    resolution TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE admin_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL,
    target_type VARCHAR(20),
    target_id BIGINT,
    details JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- O. Browsing History
-- =============================================

CREATE TABLE browsing_histories (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    store_id BIGINT REFERENCES stores(id) ON DELETE SET NULL,
    review_id BIGINT REFERENCES reviews(id) ON DELETE SET NULL,
    post_id BIGINT REFERENCES posts(id) ON DELETE SET NULL,
    viewed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================
-- P. Indexes
-- =============================================

-- Users
CREATE INDEX idx_email_verifications_token ON email_verifications(token);
CREATE INDEX idx_email_verifications_user ON email_verifications(user_id);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens(expires_at);
CREATE INDEX idx_refresh_tokens_revoked ON refresh_tokens(revoked_at);
CREATE INDEX idx_users_email ON user_auths(identifier) WHERE identity_type = 'email';
CREATE INDEX idx_users_role ON users(role);

-- Stores
CREATE INDEX idx_stores_merchant ON stores(merchant_id);
CREATE INDEX idx_stores_name ON stores(name);
CREATE INDEX idx_store_hours_store ON store_hours(store_id);
CREATE INDEX idx_store_cats_store ON store_categories(store_id);
CREATE INDEX idx_store_cats_cat ON store_categories(category_id);

-- Reviews
CREATE INDEX idx_reviews_store ON reviews(store_id);
CREATE INDEX idx_reviews_venue_date ON reviews(venue_id, created_at DESC);
CREATE INDEX idx_reviews_user ON reviews(user_id);
CREATE INDEX idx_reviews_merchant ON reviews(merchant_id);
CREATE INDEX idx_review_media_rid ON review_media(review_id);
CREATE INDEX idx_review_comments_rid ON review_comments(review_id);

-- Posts
CREATE INDEX idx_posts_user ON posts(user_id);
CREATE INDEX idx_post_comments_post ON post_comments(post_id);

-- Commerce
CREATE INDEX idx_coupons_merchant ON coupons(merchant_id);
CREATE INDEX idx_coupons_store ON coupons(store_id);
CREATE INDEX idx_packages_merchant ON packages(merchant_id);
CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_vouchers_user ON vouchers(user_id);
CREATE INDEX idx_vouchers_coupon ON vouchers(coupon_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_order ON payments(order_id);
CREATE INDEX idx_payments_user ON payments(user_id);

-- Messaging
CREATE INDEX idx_conv_participants_conv ON conversation_participants(conversation_id);
CREATE INDEX idx_conv_participants_user ON conversation_participants(user_id);
CREATE INDEX idx_messages_conv ON messages(conversation_id);
CREATE INDEX idx_messages_sender ON messages(sender_id);

-- Notifications
CREATE INDEX idx_notifications_user ON notifications(user_id);
CREATE INDEX idx_notifications_read ON notifications(user_id, is_read);

-- Reports
CREATE INDEX idx_reports_status ON reports(status);

-- Analytics
CREATE INDEX idx_analytics_merchant_date ON merchant_analytics(merchant_id, date);

-- Browsing History
CREATE INDEX idx_browsing_user ON browsing_histories(user_id);

-- +goose Down

DROP TABLE IF EXISTS browsing_histories CASCADE;
DROP TABLE IF EXISTS admin_audit_logs CASCADE;
DROP TABLE IF EXISTS reports CASCADE;
DROP TABLE IF EXISTS notifications CASCADE;
DROP TABLE IF EXISTS merchant_analytics CASCADE;
DROP TABLE IF EXISTS marketing_posts CASCADE;
DROP TABLE IF EXISTS merchant_verifications CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
DROP TABLE IF EXISTS conversation_participants CASCADE;
DROP TABLE IF EXISTS conversations CASCADE;
DROP TABLE IF EXISTS media_uploads CASCADE;
DROP TABLE IF EXISTS payments CASCADE;
DROP TABLE IF EXISTS vouchers CASCADE;
DROP TABLE IF EXISTS orders CASCADE;
DROP TABLE IF EXISTS coupons CASCADE;
DROP TABLE IF EXISTS packages CASCADE;
DROP TABLE IF EXISTS post_comments CASCADE;
DROP TABLE IF EXISTS post_tags CASCADE;
DROP TABLE IF EXISTS posts CASCADE;
DROP TABLE IF EXISTS review_votes CASCADE;
DROP TABLE IF EXISTS review_comments CASCADE;
DROP TABLE IF EXISTS review_media CASCADE;
DROP TABLE IF EXISTS review_tags CASCADE;
DROP TABLE IF EXISTS reviews CASCADE;
DROP TABLE IF EXISTS tags CASCADE;
DROP TABLE IF EXISTS store_hours CASCADE;
DROP TABLE IF EXISTS store_categories CASCADE;
DROP TABLE IF EXISTS stores CASCADE;
DROP TABLE IF EXISTS categories CASCADE;
DROP TABLE IF EXISTS merchant_follows CASCADE;
DROP TABLE IF EXISTS user_follows CASCADE;
DROP TABLE IF EXISTS likes CASCADE;
DROP TABLE IF EXISTS favorites CASCADE;
DROP TABLE IF EXISTS account_deletions CASCADE;
DROP TABLE IF EXISTS user_notifications CASCADE;
DROP TABLE IF EXISTS user_privacies CASCADE;
DROP TABLE IF EXISTS user_addresses CASCADE;
DROP TABLE IF EXISTS refresh_tokens CASCADE;
DROP TABLE IF EXISTS email_verifications CASCADE;
DROP TABLE IF EXISTS user_profiles CASCADE;
DROP TABLE IF EXISTS user_auths CASCADE;
DROP TABLE IF EXISTS merchants CASCADE;
DROP TABLE IF EXISTS users CASCADE;
