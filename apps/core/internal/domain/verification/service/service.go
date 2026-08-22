package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"github.com/revieu-corp/revieu-core-api-go/apps/core/pkg/database"
	"gorm.io/gorm"
)

type VerificationService struct {
	db *gorm.DB
}

type SubmitVerificationInput struct {
	DocumentType    string `json:"document_type"`
	DocumentURL     string `json:"document_url"`
	BusinessLicense string `json:"business_license"`
}

type VerificationStatusView struct {
	ID              int64  `json:"id"`
	MerchantID      int64  `json:"merchant_id"`
	Status          string `json:"status"`
	MerchantStatus  string `json:"merchant_status"`
	DocumentType    string `json:"document_type"`
	DocumentURL     string `json:"document_url"`
	BusinessLicense string `json:"business_license"`
	RejectionReason string `json:"rejection_reason"`
}

var ErrVerificationInvalidInput = errors.New("verification invalid input")
var ErrVerificationForbidden = errors.New("verification forbidden")

func NewVerificationService(db *gorm.DB) *VerificationService {
	if db == nil {
		db = database.DB
	}
	return &VerificationService{db: db}
}

func (s *VerificationService) Submit(ctx context.Context, userID int64, input SubmitVerificationInput) (*VerificationStatusView, error) {
	documentType, documentURL, businessLicense, err := validateSubmissionInput(input)
	if err != nil {
		return nil, ErrVerificationInvalidInput
	}

	merchant, err := s.ensureMerchant(ctx, userID)
	if err != nil {
		return nil, err
	}

	var verification model.MerchantVerification
	err = s.db.WithContext(ctx).
		Where("merchant_id = ?", merchant.ID).
		Order("id desc").
		First(&verification).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		verification = model.MerchantVerification{
			MerchantID:      merchant.ID,
			DocumentType:    documentType,
			DocumentURL:     documentURL,
			BusinessLicense: businessLicense,
			Status:          "pending",
		}
		if err := s.db.WithContext(ctx).Create(&verification).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		verification.DocumentType = documentType
		verification.DocumentURL = documentURL
		verification.BusinessLicense = businessLicense
		verification.Status = "pending"
		verification.RejectionReason = ""
		if err := s.db.WithContext(ctx).Save(&verification).Error; err != nil {
			return nil, err
		}
	}

	if err := s.db.WithContext(ctx).
		Model(&model.Merchant{}).
		Where("id = ?", merchant.ID).
		Update("verification_status", "pending").Error; err != nil {
		return nil, err
	}
	merchant.VerificationStatus = "pending"

	view := mapVerificationView(verification, merchant)
	return &view, nil
}

func (s *VerificationService) Status(ctx context.Context, userID int64) (*VerificationStatusView, error) {
	merchant, err := s.ensureMerchant(ctx, userID)
	if err != nil {
		return nil, err
	}

	var verification model.MerchantVerification
	err = s.db.WithContext(ctx).
		Where("merchant_id = ?", merchant.ID).
		Order("id desc").
		First(&verification).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		view := VerificationStatusView{
			MerchantID:     merchant.ID,
			Status:         merchant.VerificationStatus,
			MerchantStatus: merchant.VerificationStatus,
		}
		return &view, nil
	}
	if err != nil {
		return nil, err
	}

	view := mapVerificationView(verification, merchant)
	return &view, nil
}

func (s *VerificationService) ensureMerchant(ctx context.Context, userID int64) (*model.Merchant, error) {
	var user model.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVerificationForbidden
		}
		return nil, err
	}
	if user.Status != 0 || strings.ToLower(strings.TrimSpace(user.Role)) != "merchant" {
		return nil, ErrVerificationForbidden
	}

	var merchant model.Merchant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&merchant).Error; err == nil {
		return &merchant, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	merchant = model.Merchant{
		UserID:             &userID,
		Name:               "merchant",
		VerificationStatus: "unverified",
	}
	if err := s.db.WithContext(ctx).Create(&merchant).Error; err != nil {
		return nil, err
	}
	return &merchant, nil
}

func validateSubmissionInput(input SubmitVerificationInput) (string, string, string, error) {
	documentType := strings.TrimSpace(input.DocumentType)
	documentURL := strings.TrimSpace(input.DocumentURL)
	businessLicense := strings.TrimSpace(input.BusinessLicense)
	if documentType == "" || documentURL == "" || len(documentType) > 50 || len(documentURL) > 512 || len(businessLicense) > 255 {
		return "", "", "", ErrVerificationInvalidInput
	}

	parsedURL, err := url.ParseRequestURI(documentURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return "", "", "", ErrVerificationInvalidInput
	}
	return documentType, documentURL, businessLicense, nil
}

func mapVerificationView(verification model.MerchantVerification, merchant *model.Merchant) VerificationStatusView {
	merchantStatus := ""
	if merchant != nil {
		merchantStatus = merchant.VerificationStatus
	}
	return VerificationStatusView{
		ID:              verification.ID,
		MerchantID:      verification.MerchantID,
		Status:          verification.Status,
		MerchantStatus:  merchantStatus,
		DocumentType:    verification.DocumentType,
		DocumentURL:     verification.DocumentURL,
		BusinessLicense: verification.BusinessLicense,
		RejectionReason: verification.RejectionReason,
	}
}
