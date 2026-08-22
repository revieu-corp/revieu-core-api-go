package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/domain/media/dto"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/storage"
	"gorm.io/gorm"
)

var (
	ErrTooManyFiles       = errors.New("too many files, maximum 10 allowed")
	ErrInvalidContentType = errors.New("invalid content type, only image/jpeg, image/png, image/gif, image/webp allowed")
	ErrUnauthorized       = errors.New("authenticated user is required")
)

var allowedContentTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

type MediaService struct {
	db       *gorm.DB
	r2Client *storage.R2Client
}

func NewMediaService(db *gorm.DB, r2Client *storage.R2Client) *MediaService {
	if db == nil {
		db = database.DB
	}
	return &MediaService{db: db, r2Client: r2Client}
}

func (s *MediaService) CreatePresignedURLs(ctx context.Context, userID int64, req *dto.PresignedURLRequest) (*dto.PresignedURLResponse, error) {
	if userID <= 0 {
		return nil, ErrUnauthorized
	}
	if len(req.Files) > 10 {
		return nil, ErrTooManyFiles
	}

	if len(req.Files) == 0 {
		return &dto.PresignedURLResponse{Uploads: []dto.UploadInfo{}}, nil
	}

	response := &dto.PresignedURLResponse{
		Uploads: make([]dto.UploadInfo, 0, len(req.Files)),
	}

	now := time.Now()
	year := now.Format("2006")
	month := now.Format("01")

	for _, file := range req.Files {
		ext, ok := allowedContentTypes[strings.ToLower(file.ContentType)]
		if !ok {
			return nil, ErrInvalidContentType
		}

		fileUUID := uuid.New().String()
		objectKey := fmt.Sprintf("uploads/%s/%s/%s%s", year, month, fileUUID, ext)

		result, err := s.r2Client.GeneratePresignedURL(ctx, objectKey, file.ContentType)
		if err != nil {
			return nil, fmt.Errorf("failed to generate presigned URL for %s: %w", file.Filename, err)
		}

		upload := model.MediaUpload{
			UUID:      fileUUID,
			UserID:    userID,
			ObjectKey: objectKey,
			FileURL:   result.FileURL,
			Status:    "pending",
		}

		if err := s.db.WithContext(ctx).Create(&upload).Error; err != nil {
			return nil, fmt.Errorf("failed to save upload record: %w", err)
		}

		response.Uploads = append(response.Uploads, dto.UploadInfo{
			ID:        fileUUID,
			UploadURL: result.UploadURL,
			FileURL:   result.FileURL,
			ExpiresAt: result.ExpiresAt,
		})
	}

	return response, nil
}

func (s *MediaService) CreateUpload(ctx context.Context, userID int64) (model.MediaUpload, error) {
	if userID <= 0 {
		return model.MediaUpload{}, ErrUnauthorized
	}
	upload := model.MediaUpload{
		UUID:      uuid.New().String(),
		UserID:    userID,
		ObjectKey: fmt.Sprintf("uploads/%d", time.Now().UnixNano()),
		FileURL:   fmt.Sprintf("https://example.com/files/%d", time.Now().UnixNano()),
		Status:    "pending",
	}
	return upload, s.db.WithContext(ctx).Create(&upload).Error
}

func (s *MediaService) Analyze(ctx context.Context, userID, id int64) error {
	if userID <= 0 {
		return ErrUnauthorized
	}
	var upload model.MediaUpload
	return s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&upload).Error
}
