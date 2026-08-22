package model

import "time"

// OperationalAuditLog records business writes that must be traceable without
// depending on application log retention alone.
type OperationalAuditLog struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ActorID    int64     `gorm:"not null;index" json:"actor_id"`
	ActorRole  string    `gorm:"type:varchar(30);not null" json:"actor_role"`
	Action     string    `gorm:"type:varchar(80);not null;index" json:"action"`
	TargetType string    `gorm:"type:varchar(30);not null" json:"target_type"`
	TargetID   int64     `gorm:"index" json:"target_id"`
	Result     string    `gorm:"type:varchar(20);not null;index" json:"result"`
	ErrorClass string    `gorm:"type:varchar(30)" json:"error_class,omitempty"`
	Details    string    `gorm:"type:jsonb;default:'{}'" json:"details"`
	DurationMS int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

func (o *OperationalAuditLog) TableName() string { return "operational_audit_logs" }
