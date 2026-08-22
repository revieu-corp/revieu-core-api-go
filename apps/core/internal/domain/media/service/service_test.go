package service

import (
	"context"
	"errors"
	"testing"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/testutil"
	"gorm.io/gorm"
)

func TestCreateUploadBindsAuthenticatedUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewMediaService(db, nil)

	upload, err := svc.CreateUpload(context.Background(), 42)
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if upload.UserID != 42 {
		t.Fatalf("upload owner = %d, want 42", upload.UserID)
	}

	var stored model.MediaUpload
	if err := db.First(&stored, upload.ID).Error; err != nil {
		t.Fatalf("reload upload: %v", err)
	}
	if stored.UserID != 42 {
		t.Fatalf("stored upload owner = %d, want 42", stored.UserID)
	}
}

func TestMediaOperationsRejectMissingOwnerAndProtectAnalysis(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewMediaService(db, nil)

	if _, err := svc.CreateUpload(context.Background(), 0); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("create without owner error = %v, want %v", err, ErrUnauthorized)
	}
	upload, err := svc.CreateUpload(context.Background(), 42)
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if err := svc.Analyze(context.Background(), 0, upload.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("analyze without owner error = %v, want %v", err, ErrUnauthorized)
	}
	if err := svc.Analyze(context.Background(), 43, upload.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user analyze error = %v, want record not found", err)
	}
	if err := svc.Analyze(context.Background(), 42, upload.ID); err != nil {
		t.Fatalf("owner analyze: %v", err)
	}
}
