package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

var (
	ErrAdminInvalidInput = errors.New("invalid admin input")
	ErrAdminNotFound     = errors.New("admin resource not found")
	ErrAdminInvalidState = errors.New("invalid admin state transition")
)

var reportStatuses = map[string]struct{}{
	"pending":      {},
	"under_review": {},
	"resolved":     {},
	"dismissed":    {},
}

var merchantVerificationStatuses = map[string]struct{}{
	"unverified":        {},
	"pending":           {},
	"under_review":      {},
	"verified":          {},
	"rejected":          {},
	"resubmit_required": {},
}

type AdminService struct {
	db *gorm.DB
}

type ListReportsQuery struct {
	Status string
	Limit  int
	Cursor int64
}

type ListMerchantsQuery struct {
	Status             *int16
	VerificationStatus string
	Limit              int
	Cursor             int64
}

type UpdateReportInput struct {
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
}

type UpdateMerchantInput struct {
	Status             *int16  `json:"status"`
	VerificationStatus *string `json:"verification_status"`
}

type ReportPage struct {
	Data   []model.Report `json:"data"`
	Cursor *int64         `json:"cursor,omitempty"`
}

type MerchantPage struct {
	Data   []model.Merchant `json:"data"`
	Cursor *int64           `json:"cursor,omitempty"`
}

func NewAdminService(db *gorm.DB) *AdminService {
	if db == nil {
		db = database.DB
	}
	return &AdminService{db: db}
}

func (s *AdminService) ListReports(ctx context.Context, query ListReportsQuery) (*ReportPage, error) {
	limit, err := normalizePageSize(query.Limit)
	if err != nil || query.Cursor < 0 {
		return nil, ErrAdminInvalidInput
	}

	status := strings.ToLower(strings.TrimSpace(query.Status))
	if status != "" {
		if _, ok := reportStatuses[status]; !ok {
			return nil, ErrAdminInvalidInput
		}
	}

	db := s.db.WithContext(ctx).Model(&model.Report{})
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if query.Cursor > 0 {
		db = db.Where("id < ?", query.Cursor)
	}

	rows := make([]model.Report, 0, limit)
	if err := db.Order("created_at desc").Order("id desc").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}

	page := &ReportPage{Data: rows}
	if len(rows) > limit {
		rows = rows[:limit]
		page.Data = rows
		cursor := rows[len(rows)-1].ID
		page.Cursor = &cursor
	}
	return page, nil
}

func (s *AdminService) UpdateReport(ctx context.Context, adminID, reportID int64, input UpdateReportInput) (*model.Report, error) {
	if adminID <= 0 || reportID <= 0 {
		return nil, ErrAdminInvalidInput
	}

	status := strings.ToLower(strings.TrimSpace(input.Status))
	resolution := strings.TrimSpace(input.Resolution)
	if _, ok := reportStatuses[status]; !ok || len(resolution) > 10000 {
		return nil, ErrAdminInvalidInput
	}
	if (status == "resolved" || status == "dismissed") && resolution == "" {
		return nil, ErrAdminInvalidInput
	}

	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			tx.Rollback()
			panic(recovered)
		}
	}()

	var report model.Report
	if err := tx.First(&report, reportID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdminNotFound
		}
		return nil, err
	}
	if report.Status == "resolved" || report.Status == "dismissed" {
		tx.Rollback()
		return nil, ErrAdminInvalidState
	}

	now := time.Now().UTC()
	report.Status = status
	report.ReviewedBy = &adminID
	report.ReviewedAt = &now
	report.Resolution = resolution
	if err := tx.Save(&report).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := createAuditLog(tx, adminID, "resolve_report", "report", report.ID, map[string]interface{}{
		"status":     status,
		"resolution": resolution,
	}); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &report, nil
}

func (s *AdminService) ListMerchants(ctx context.Context, query ListMerchantsQuery) (*MerchantPage, error) {
	limit, err := normalizePageSize(query.Limit)
	if err != nil || query.Cursor < 0 {
		return nil, ErrAdminInvalidInput
	}

	verificationStatus := strings.ToLower(strings.TrimSpace(query.VerificationStatus))
	if verificationStatus != "" {
		if _, ok := merchantVerificationStatuses[verificationStatus]; !ok {
			return nil, ErrAdminInvalidInput
		}
	}
	if query.Status != nil && (*query.Status < 0 || *query.Status > 2) {
		return nil, ErrAdminInvalidInput
	}

	db := s.db.WithContext(ctx).Model(&model.Merchant{})
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if verificationStatus != "" {
		db = db.Where("verification_status = ?", verificationStatus)
	}
	if query.Cursor > 0 {
		db = db.Where("id < ?", query.Cursor)
	}

	rows := make([]model.Merchant, 0, limit)
	if err := db.Order("created_at desc").Order("id desc").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}

	page := &MerchantPage{Data: rows}
	if len(rows) > limit {
		rows = rows[:limit]
		page.Data = rows
		cursor := rows[len(rows)-1].ID
		page.Cursor = &cursor
	}
	return page, nil
}

func (s *AdminService) UpdateMerchant(ctx context.Context, adminID, merchantID int64, input UpdateMerchantInput) (*model.Merchant, error) {
	if adminID <= 0 || merchantID <= 0 || (input.Status == nil && input.VerificationStatus == nil) {
		return nil, ErrAdminInvalidInput
	}
	if input.Status != nil && (*input.Status < 0 || *input.Status > 2) {
		return nil, ErrAdminInvalidInput
	}

	var verificationStatus string
	if input.VerificationStatus != nil {
		verificationStatus = strings.ToLower(strings.TrimSpace(*input.VerificationStatus))
		if _, ok := merchantVerificationStatuses[verificationStatus]; !ok {
			return nil, ErrAdminInvalidInput
		}
	}

	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			tx.Rollback()
			panic(recovered)
		}
	}()

	var merchant model.Merchant
	if err := tx.First(&merchant, merchantID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdminNotFound
		}
		return nil, err
	}

	if input.Status != nil {
		merchant.Status = *input.Status
	}
	if input.VerificationStatus != nil {
		merchant.VerificationStatus = verificationStatus
		if verificationStatus == "verified" {
			now := time.Now().UTC()
			merchant.VerifiedAt = &now
		} else {
			merchant.VerifiedAt = nil
		}
	}
	if err := tx.Save(&merchant).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	details := map[string]interface{}{}
	if input.Status != nil {
		details["status"] = *input.Status
	}
	if input.VerificationStatus != nil {
		details["verification_status"] = verificationStatus
	}
	if err := createAuditLog(tx, adminID, "update_merchant", "merchant", merchant.ID, details); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &merchant, nil
}

func normalizePageSize(limit int) (int, error) {
	if limit == 0 {
		return DefaultPageSize, nil
	}
	if limit < 1 || limit > MaxPageSize {
		return 0, ErrAdminInvalidInput
	}
	return limit, nil
}

func createAuditLog(tx *gorm.DB, adminID int64, action, targetType string, targetID int64, details map[string]interface{}) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	return tx.Create(&model.AdminAuditLog{
		AdminID:    adminID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Details:    string(payload),
	}).Error
}
