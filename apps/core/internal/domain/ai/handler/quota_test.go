package handler

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newQuotaTestStore(t *testing.T, cfg config.AIQuotaConfig) *quotaStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AIUsageWindow{}); err != nil {
		t.Fatalf("migrate quota table: %v", err)
	}
	return newQuotaStore(db, cfg)
}

func TestQuotaStoreEnforcesUserRateLimit(t *testing.T) {
	store := newQuotaTestStore(t, config.AIQuotaConfig{UserPerMinute: 2})
	now := time.Date(2026, time.August, 22, 12, 34, 10, 0, time.UTC)

	for i := 0; i < 2; i++ {
		if err := store.allow(context.Background(), 7, "203.0.113.7", now); err != nil {
			t.Fatalf("request %d should pass: %v", i+1, err)
		}
	}

	err := store.allow(context.Background(), 7, "203.0.113.7", now)
	if !errors.Is(err, ErrAIUserRateLimited) {
		t.Fatalf("expected user rate limit, got %v", err)
	}
	var limitErr *quotaLimitError
	if !errors.As(err, &limitErr) || limitErr.RetryAfterSeconds() < 1 {
		t.Fatalf("expected retry metadata, got %v", err)
	}

	if err := store.allow(context.Background(), 7, "203.0.113.7", now.Add(time.Minute)); err != nil {
		t.Fatalf("new minute should pass: %v", err)
	}
}

func TestQuotaStoreEnforcesDailyQuotaAndRollsBackPartialCounters(t *testing.T) {
	store := newQuotaTestStore(t, config.AIQuotaConfig{DailyPerUser: 1, MonthlyPerUser: 1})
	now := time.Date(2026, time.August, 22, 12, 34, 10, 0, time.UTC)

	if err := store.allow(context.Background(), 7, "203.0.113.7", now); err != nil {
		t.Fatalf("first request should pass: %v", err)
	}
	err := store.allow(context.Background(), 7, "203.0.113.7", now.Add(time.Minute))
	if !errors.Is(err, ErrAIDailyQuota) {
		t.Fatalf("expected daily quota, got %v", err)
	}

	var rows []model.AIUsageWindow
	if err := store.db.Find(&rows).Error; err != nil {
		t.Fatalf("read counters: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("failed request must not create partial counters, got %d rows", len(rows))
	}
}

func TestQuotaStoreEnforcesIPAndGlobalLimits(t *testing.T) {
	store := newQuotaTestStore(t, config.AIQuotaConfig{IPPerMinute: 1, GlobalPerMinute: 1})
	now := time.Date(2026, time.August, 22, 12, 34, 10, 0, time.UTC)

	if err := store.allow(context.Background(), 7, "203.0.113.7", now); err != nil {
		t.Fatalf("first request should pass: %v", err)
	}
	err := store.allow(context.Background(), 8, "203.0.113.7", now)
	if !errors.Is(err, ErrAIGlobalLimited) {
		t.Fatalf("global limit is checked before IP limit, got %v", err)
	}

	store = newQuotaTestStore(t, config.AIQuotaConfig{IPPerMinute: 1})
	if err := store.allow(context.Background(), 7, "203.0.113.7", now); err != nil {
		t.Fatalf("first request should pass: %v", err)
	}
	err = store.allow(context.Background(), 8, "203.0.113.7", now)
	if !errors.Is(err, ErrAIIPRateLimited) {
		t.Fatalf("expected IP limit, got %v", err)
	}
}

func TestHashClientIPIsOpaque(t *testing.T) {
	hash := hashClientIP("203.0.113.7")
	if hash == "203.0.113.7" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(hash) {
		t.Fatalf("expected opaque SHA-256 client key, got %q", hash)
	}
}
