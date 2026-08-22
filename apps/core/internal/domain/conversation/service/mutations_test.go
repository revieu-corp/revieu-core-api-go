package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
)

func TestConversationMutationsPersistAndRemainScopedToParticipant(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewConversationService(db)

	owner := model.User{Role: "user", Status: 0}
	other := model.User{Role: "user", Status: 0}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	conversation := model.Conversation{Type: "direct", Title: "Customer Chat", UpdatedAt: time.Now().UTC()}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := db.Create(&[]model.ConversationParticipant{
		{ConversationID: conversation.ID, UserID: owner.ID, Role: "owner"},
		{ConversationID: conversation.ID, UserID: other.ID, Role: "member"},
	}).Error; err != nil {
		t.Fatalf("create participants: %v", err)
	}
	if err := db.Create(&model.Message{
		ConversationID: conversation.ID,
		SenderID:       other.ID,
		Content:        "hello",
		MessageType:    "text",
		CreatedAt:      time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	updated, err := svc.UpdateSettings(context.Background(), owner.ID, conversation.ID, UpdateConversationSettingsInput{IsPinned: boolPtr(true)})
	if err != nil {
		t.Fatalf("pin conversation: %v", err)
	}
	if !updated.IsPinned {
		t.Fatalf("expected pinned response, got %#v", updated)
	}
	listed, err := svc.List(context.Background(), owner.ID)
	if err != nil || len(listed) != 1 || !listed[0].IsPinned {
		t.Fatalf("pinned state did not survive list: %#v, %v", listed, err)
	}

	if err := svc.ClearMessages(context.Background(), owner.ID, conversation.ID); err != nil {
		t.Fatalf("clear messages: %v", err)
	}
	var messageCount int64
	if err := db.Model(&model.Message{}).Where("conversation_id = ?", conversation.ID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count cleared messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("expected no messages after clear, got %d", messageCount)
	}

	if err := svc.Delete(context.Background(), owner.ID, conversation.ID); err != nil {
		t.Fatalf("delete conversation membership: %v", err)
	}
	listed, err = svc.List(context.Background(), owner.ID)
	if err != nil || len(listed) != 0 {
		t.Fatalf("deleted conversation remained for owner: %#v, %v", listed, err)
	}
	listed, err = svc.List(context.Background(), other.ID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("delete should remain scoped to owner: %#v, %v", listed, err)
	}
	if err := svc.Delete(context.Background(), owner.ID, conversation.ID); !errors.Is(err, ErrConversationForbidden) {
		t.Fatalf("expected repeat delete to be forbidden, got %v", err)
	}
	if err := svc.ClearMessages(context.Background(), int64(9999), conversation.ID); !errors.Is(err, ErrConversationForbidden) {
		t.Fatalf("expected non-member clear to be forbidden, got %v", err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
