// Package observability centralizes the minimum operational evidence required
// for high-value business writes. It deliberately uses the existing structured
// logger so deployments do not need a new metrics dependency to get useful
// counters and latency samples.
package observability

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/logger"
	"gorm.io/gorm"
	"log/slog"
)

const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

type AuditInput struct {
	ActorID    int64
	ActorRole  string
	Action     string
	TargetType string
	TargetID   int64
	Result     string
	ErrorClass string
	Details    string
	Duration   time.Duration
}

// WriteAuditTx writes an audit event using the caller's transaction. Audit
// callers intentionally decide whether an audit failure is fatal; operational
// telemetry must not turn a successful payment into a failed payment.
func WriteAuditTx(ctx context.Context, tx *gorm.DB, input AuditInput) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	return tx.WithContext(ctx).Create(&model.OperationalAuditLog{
		ActorID:    input.ActorID,
		ActorRole:  input.ActorRole,
		Action:     input.Action,
		TargetType: input.TargetType,
		TargetID:   input.TargetID,
		Result:     input.Result,
		ErrorClass: input.ErrorClass,
		Details:    normalizeDetails(input.Details),
		DurationMS: input.Duration.Milliseconds(),
	}).Error
}

func WriteAudit(ctx context.Context, db *gorm.DB, input AuditInput) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	return WriteAuditTx(ctx, db, input)
}

// RecordTransaction emits one searchable counter event and one latency sample
// as structured JSON. The log keys are stable so they can later be collected
// by any metrics backend without changing the business packages.
func RecordTransaction(ctx context.Context, action string, success bool, err error, duration time.Duration) {
	result := ResultFailure
	if success {
		result = ResultSuccess
	}
	errorClass := "none"
	if err != nil {
		errorClass = ClassifyError(err)
	}

	levelLogger := logger.Info
	if !success {
		levelLogger = logger.Warn
	}
	levelLogger(ctx, "transaction completed",
		slog.String("metric_name", "revieu_transaction_total"),
		slog.String("metric_type", "counter"),
		slog.String("action", action),
		slog.String("result", result),
		slog.String("error_class", errorClass),
		slog.Int64("duration_ms", duration.Milliseconds()),
	)
	logger.Info(ctx, "transaction latency",
		slog.String("metric_name", "revieu_transaction_duration_ms"),
		slog.String("metric_type", "histogram"),
		slog.String("action", action),
		slog.String("result", result),
		slog.Int64("duration_ms", duration.Milliseconds()),
	)
}

// ClassifyError keeps dashboards stable while preserving the original error
// for the API caller and application logs.
func ClassifyError(err error) string {
	if err == nil {
		return "none"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unauthorized"), strings.Contains(message, "forbidden"):
		return "forbidden"
	case strings.Contains(message, "not found"):
		return "not_found"
	case strings.Contains(message, "inactive"), strings.Contains(message, "expired"), strings.Contains(message, "sold out"), strings.Contains(message, "not redeemable"), strings.Contains(message, "state"), strings.Contains(message, "not published"), strings.Contains(message, "mismatch"), strings.Contains(message, "limit exceeded"):
		return "business_rule"
	case strings.Contains(message, "invalid"), strings.Contains(message, "required"), strings.Contains(message, "empty"):
		return "validation"
	default:
		return "internal"
	}
}

func normalizeDetails(details string) string {
	details = strings.TrimSpace(details)
	if details == "" || !json.Valid([]byte(details)) {
		return "{}"
	}
	return details
}
