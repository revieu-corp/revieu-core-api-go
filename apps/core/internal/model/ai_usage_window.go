package model

import "time"

// AIUsageWindow is an atomic counter for a configured AI quota window.
// Scope keys are deliberately opaque for IP-based windows; callers store a
// digest rather than a raw client address.
type AIUsageWindow struct {
	Scope        string    `gorm:"primaryKey;size:32"`
	ScopeKey     string    `gorm:"primaryKey;size:128"`
	WindowStart  time.Time `gorm:"primaryKey;index"`
	RequestCount int       `gorm:"not null;default:0"`
}

func (AIUsageWindow) TableName() string { return "ai_usage_windows" }
