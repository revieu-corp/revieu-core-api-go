package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWriteAuditPersistsStructuredEvent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.OperationalAuditLog{}); err != nil {
		t.Fatalf("migrate audit table: %v", err)
	}

	err = WriteAudit(context.Background(), db, AuditInput{
		ActorID:    42,
		ActorRole:  "merchant",
		Action:     "coupon.activate",
		TargetType: "coupon",
		TargetID:   7,
		Result:     ResultSuccess,
		Details:    `{"status":"active"}`,
		Duration:   1250 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("write audit: %v", err)
	}

	var got model.OperationalAuditLog
	if err := db.First(&got).Error; err != nil {
		t.Fatalf("load audit: %v", err)
	}
	if got.ActorID != 42 || got.Action != "coupon.activate" || got.Result != ResultSuccess || got.DurationMS != 1250 {
		t.Fatalf("unexpected audit row: %+v", got)
	}
}

func TestWriteAuditNormalizesInvalidDetails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.OperationalAuditLog{}); err != nil {
		t.Fatalf("migrate audit table: %v", err)
	}
	if err := WriteAudit(context.Background(), db, AuditInput{Details: "not-json"}); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	var got model.OperationalAuditLog
	if err := db.First(&got).Error; err != nil {
		t.Fatalf("load audit: %v", err)
	}
	if got.Details != "{}" {
		t.Fatalf("expected normalized details, got %q", got.Details)
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "none", want: "none"},
		{name: "forbidden", err: errors.New("order forbidden"), want: "forbidden"},
		{name: "not found", err: errors.New("coupon not found"), want: "not_found"},
		{name: "validation", err: errors.New("invalid order input"), want: "validation"},
		{name: "business", err: errors.New("coupon expired"), want: "business_rule"},
		{name: "invalid state business", err: errors.New("invalid order state"), want: "business_rule"},
		{name: "internal", err: errors.New("database unavailable"), want: "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyError(tc.err); got != tc.want {
				t.Fatalf("ClassifyError(%v)=%q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
