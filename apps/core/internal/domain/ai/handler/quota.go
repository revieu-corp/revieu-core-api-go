package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAIUserRateLimited       = errors.New("ai: user rate limit exceeded")
	ErrAIIPRateLimited         = errors.New("ai: IP rate limit exceeded")
	ErrAIGlobalLimited         = errors.New("ai: global rate limit exceeded")
	ErrAIDailyQuota            = errors.New("ai: daily quota exceeded")
	ErrAIMonthlyQuota          = errors.New("ai: monthly quota exceeded")
	ErrAIQuotaStoreUnavailable = errors.New("ai: quota store unavailable")
)

type quotaLimitError struct {
	reason     error
	retryAfter time.Duration
}

func (e *quotaLimitError) Error() string { return e.reason.Error() }
func (e *quotaLimitError) Unwrap() error { return e.reason }

func (e *quotaLimitError) RetryAfterSeconds() int {
	seconds := int(e.retryAfter / time.Second)
	if e.retryAfter%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}

type quotaWindow struct {
	start time.Time
	count int
}

type memoryUserQuota struct {
	minute quotaWindow
	day    quotaWindow
	month  quotaWindow
}

// quotaStore uses database counters in production so the limits survive
// process restarts and are shared by application replicas. The memory path is
// only a safe fallback for isolated handler tests that do not configure a DB.
type quotaStore struct {
	db     *gorm.DB
	cfg    config.AIQuotaConfig
	mu     sync.Mutex
	users  map[int64]*memoryUserQuota
	ips    map[string]quotaWindow
	global quotaWindow
}

func newQuotaStore(db *gorm.DB, cfg config.AIQuotaConfig) *quotaStore {
	return &quotaStore{
		db:    db,
		cfg:   cfg,
		users: make(map[int64]*memoryUserQuota),
		ips:   make(map[string]quotaWindow),
	}
}

func (s *quotaStore) allow(ctx context.Context, userID int64, clientIP string, now time.Time) error {
	if userID <= 0 {
		return errors.New("ai: authenticated user is required")
	}
	if s.db == nil {
		return s.allowMemory(userID, clientIP, now)
	}

	now = now.UTC()
	minuteStart := now.Truncate(time.Minute)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	ipKey := hashClientIP(clientIP)
	userKey := strconv.FormatInt(userID, 10)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := consumeWindow(tx, "global_minute", "global", minuteStart, s.cfg.GlobalPerMinute, ErrAIGlobalLimited, retryUntil(minuteStart, now)); err != nil {
			return err
		}
		if err := consumeWindow(tx, "ip_minute", ipKey, minuteStart, s.cfg.IPPerMinute, ErrAIIPRateLimited, retryUntil(minuteStart, now)); err != nil {
			return err
		}
		if err := consumeWindow(tx, "user_minute", userKey, minuteStart, s.cfg.UserPerMinute, ErrAIUserRateLimited, retryUntil(minuteStart, now)); err != nil {
			return err
		}
		if err := consumeWindow(tx, "user_day", userKey, dayStart, s.cfg.DailyPerUser, ErrAIDailyQuota, retryUntil(dayStart.Add(24*time.Hour), now)); err != nil {
			return err
		}
		return consumeWindow(tx, "user_month", userKey, monthStart, s.cfg.MonthlyPerUser, ErrAIMonthlyQuota, retryUntil(monthStart.AddDate(0, 1, 0), now))
	})
	if err == nil {
		return nil
	}
	var limitErr *quotaLimitError
	if errors.As(err, &limitErr) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrAIQuotaStoreUnavailable, err)
}

func consumeWindow(tx *gorm.DB, scope, key string, start time.Time, limit int, reason error, retryAfter time.Duration) error {
	if limit <= 0 {
		return nil
	}
	row := model.AIUsageWindow{Scope: scope, ScopeKey: key, WindowStart: start, RequestCount: 1}
	result := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "scope"}, {Name: "scope_key"}, {Name: "window_start"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"request_count": gorm.Expr("request_count + ?", 1),
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "request_count < ?", Vars: []interface{}{limit}},
		}},
	}).Create(&row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return &quotaLimitError{reason: reason, retryAfter: retryAfter}
	}
	return nil
}

func (s *quotaStore) allowMemory(userID int64, clientIP string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now = now.UTC()
	minuteStart := now.Truncate(time.Minute)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	ipKey := hashClientIP(clientIP)
	user := s.users[userID]
	if user == nil {
		user = &memoryUserQuota{}
		s.users[userID] = user
	}

	minute := advanceWindow(user.minute, minuteStart)
	day := advanceWindow(user.day, dayStart)
	month := advanceWindow(user.month, monthStart)
	ip := advanceWindow(s.ips[ipKey], minuteStart)
	global := advanceWindow(s.global, minuteStart)

	if err := checkLimit(global.count, s.cfg.GlobalPerMinute, ErrAIGlobalLimited, retryUntil(minuteStart, now)); err != nil {
		return err
	}
	if err := checkLimit(ip.count, s.cfg.IPPerMinute, ErrAIIPRateLimited, retryUntil(minuteStart, now)); err != nil {
		return err
	}
	if err := checkLimit(minute.count, s.cfg.UserPerMinute, ErrAIUserRateLimited, retryUntil(minuteStart, now)); err != nil {
		return err
	}
	if err := checkLimit(day.count, s.cfg.DailyPerUser, ErrAIDailyQuota, retryUntil(dayStart.Add(24*time.Hour), now)); err != nil {
		return err
	}
	if err := checkLimit(month.count, s.cfg.MonthlyPerUser, ErrAIMonthlyQuota, retryUntil(monthStart.AddDate(0, 1, 0), now)); err != nil {
		return err
	}

	global.count++
	ip.count++
	minute.count++
	day.count++
	month.count++
	s.global, s.ips[ipKey] = global, ip
	user.minute, user.day, user.month = minute, day, month
	return nil
}

func checkLimit(count, limit int, reason error, retryAfter time.Duration) error {
	if limit > 0 && count >= limit {
		return &quotaLimitError{reason: reason, retryAfter: retryAfter}
	}
	return nil
}

func advanceWindow(window quotaWindow, start time.Time) quotaWindow {
	if !window.start.Equal(start) {
		return quotaWindow{start: start}
	}
	return window
}

func retryUntil(target, now time.Time) time.Duration {
	if target.Before(now) {
		return time.Second
	}
	return target.Sub(now)
}

func hashClientIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	digest := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(digest[:])
}
