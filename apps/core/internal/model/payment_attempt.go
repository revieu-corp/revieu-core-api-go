package model

import "time"

// PaymentAttempt is the durable, gateway-agnostic state record for one
// idempotent order-payment execution. It is separate from Payment so a
// failed/processing attempt remains diagnosable without changing the existing
// payment response contract.
type PaymentAttempt struct {
	ID                int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	OrderID           int64      `gorm:"not null;index;uniqueIndex:idx_payment_attempt_order_key" json:"order_id"`
	UserID            int64      `gorm:"not null;index" json:"user_id"`
	IdempotencyKey    string     `gorm:"type:varchar(255);not null;uniqueIndex:idx_payment_attempt_order_key" json:"idempotency_key"`
	Status            string     `gorm:"type:varchar(20);not null;index" json:"status"`
	Amount            float64    `gorm:"type:numeric(10,2);not null" json:"amount"`
	Currency          string     `gorm:"type:varchar(10);not null" json:"currency"`
	PaymentMethod     string     `gorm:"type:varchar(30)" json:"payment_method"`
	ProviderReference string     `gorm:"type:varchar(255)" json:"provider_reference,omitempty"`
	PaymentID         *int64     `gorm:"index" json:"payment_id,omitempty"`
	FailureReason     string     `gorm:"type:text" json:"failure_reason,omitempty"`
	StartedAt         time.Time  `gorm:"not null;index" json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (p *PaymentAttempt) TableName() string { return "payment_attempts" }
