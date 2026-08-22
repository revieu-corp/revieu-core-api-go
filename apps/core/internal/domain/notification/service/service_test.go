package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func TestCreateEventIsIdempotentAndPersistsPayload(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	input := EventInput{
		RecipientID: 1,
		Type:        model.NotificationTypeOrderPaid,
		Title:       "Payment successful",
		Content:     "Order is ready.",
		Data:        map[string]interface{}{"order_id": int64(42)},
		DedupKey:    "order_paid:42",
	}

	created, inserted, err := CreateEventTx(ctx, db, input)
	if err != nil {
		t.Fatalf("first event failed: %v", err)
	}
	if !inserted || created.ID == 0 {
		t.Fatalf("expected first event to be inserted, got inserted=%v id=%d", inserted, created.ID)
	}
	createdAgain, insertedAgain, err := CreateEventTx(ctx, db, input)
	if err != nil {
		t.Fatalf("duplicate event failed: %v", err)
	}
	if insertedAgain || createdAgain.ID != created.ID {
		t.Fatalf("expected duplicate to reuse event %d, got inserted=%v id=%d", created.ID, insertedAgain, createdAgain.ID)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(created.Data), &payload); err != nil {
		t.Fatalf("decode notification payload: %v", err)
	}
	if payload["order_id"] != float64(42) {
		t.Fatalf("unexpected notification payload: %+v", payload)
	}
	var count int64
	if err := db.Model(&model.Notification{}).Where("dedup_key = ?", input.DedupKey).Count(&count).Error; err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one notification, got %d", count)
	}
}

func TestCreateEventRejectsUnsupportedType(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_, _, err := CreateEventTx(context.Background(), db, EventInput{
		RecipientID: 1,
		Type:        "unsupported",
		Title:       "Invalid",
	})
	if err != ErrNotificationInvalid {
		t.Fatalf("expected invalid notification type error, got %v", err)
	}
}
