package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupVoucherTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Merchant{},
		&model.Store{},
		&model.Coupon{},
		&model.Voucher{},
		&model.OperationalAuditLog{},
	); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	return db
}

func TestVoucherServiceCreateAssignsUniqueScanTokens(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	user := model.User{ID: 1001, Role: "user", Status: 0}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	merchantOwner := model.User{ID: 1002, Role: "user", Status: 0}
	if err := db.Create(&merchantOwner).Error; err != nil {
		t.Fatalf("failed to create merchant owner: %v", err)
	}

	merchant := model.Merchant{Name: "Create Merchant", UserID: &merchantOwner.ID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	store := model.Store{MerchantID: merchant.ID, Name: "Create Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &store.ID,
		Title:         "Create Coupon",
		Type:          "cash",
		TotalQuantity: 100,
		MaxPerUser:    5,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	first, err := svc.Create(context.Background(), CreateVoucherRequest{
		CouponID: strconv.FormatInt(coupon.ID, 10),
		UserID:   strconv.FormatInt(user.ID, 10),
		Code:     "CREATE-ONE",
	})
	if err != nil {
		t.Fatalf("failed to create first voucher: %v", err)
	}
	second, err := svc.Create(context.Background(), CreateVoucherRequest{
		CouponID: strconv.FormatInt(coupon.ID, 10),
		UserID:   strconv.FormatInt(user.ID, 10),
		Code:     "CREATE-TWO",
	})
	if err != nil {
		t.Fatalf("failed to create second voucher: %v", err)
	}

	if first.ScanToken == "" {
		t.Fatalf("expected first voucher scan token to be populated")
	}
	if second.ScanToken == "" {
		t.Fatalf("expected second voucher scan token to be populated")
	}
	if first.ScanToken == second.ScanToken {
		t.Fatalf("expected voucher scan tokens to be unique, got %q", first.ScanToken)
	}
}

func TestVoucherServiceRedeemByMerchantAllowsSoftDeletedCoupon(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	merchantUserID := int64(901)
	customerUserID := int64(902)
	for _, id := range []int64{merchantUserID, customerUserID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}

	merchant := model.Merchant{Name: "Merchant Owner", UserID: &merchantUserID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Redeem Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Redeem Coupon",
		Type:          "cash",
		TotalQuantity: 100,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	voucher := model.Voucher{
		Code:       "voucher-soft-delete-test",
		CouponID:   coupon.ID,
		UserID:     customerUserID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	if err := db.Delete(&coupon).Error; err != nil {
		t.Fatalf("failed to soft-delete coupon: %v", err)
	}

	if err := svc.RedeemByMerchant(context.Background(), merchantUserID, voucher.ID); err != nil {
		t.Fatalf("redeem by merchant returned error: %v", err)
	}

	var refreshedVoucher model.Voucher
	if err := db.First(&refreshedVoucher, voucher.ID).Error; err != nil {
		t.Fatalf("failed to reload voucher: %v", err)
	}
	if refreshedVoucher.Status != "used" {
		t.Fatalf("expected voucher status used, got %q", refreshedVoucher.Status)
	}
	if refreshedVoucher.RedeemedBy == nil || *refreshedVoucher.RedeemedBy != merchantUserID {
		t.Fatalf("expected redeemed_by=%d, got %+v", merchantUserID, refreshedVoucher.RedeemedBy)
	}

	var refreshedCoupon model.Coupon
	if err := db.Unscoped().First(&refreshedCoupon, coupon.ID).Error; err != nil {
		t.Fatalf("failed to reload coupon unscoped: %v", err)
	}
	if refreshedCoupon.RedeemedCount != 1 {
		t.Fatalf("expected redeemed_count=1, got %d", refreshedCoupon.RedeemedCount)
	}

}

func TestPreviewRedeemByTokenAllowsIssuingMerchant(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	merchantUserID := int64(1201)
	customerUserID := int64(1202)
	for _, id := range []int64{merchantUserID, customerUserID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}

	merchant := model.Merchant{Name: "Preview Merchant", UserID: &merchantUserID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Preview Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Preview Coupon",
		Type:          "cash",
		TotalQuantity: 100,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}

	voucher := model.Voucher{
		Code:       "voucher-preview-token",
		ScanToken:  "scan-token-preview-merchant",
		CouponID:   coupon.ID,
		UserID:     customerUserID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	preview, err := svc.PreviewRedeemByToken(context.Background(), merchantUserID, voucher.ScanToken)
	if err != nil {
		t.Fatalf("preview redeem returned error: %v", err)
	}

	if !preview.CanRedeem {
		t.Fatalf("expected preview to be redeemable")
	}
	if preview.Reason != "" {
		t.Fatalf("expected empty reason, got %q", preview.Reason)
	}
	if preview.VoucherID != voucher.ID {
		t.Fatalf("expected voucher_id=%d, got %d", voucher.ID, preview.VoucherID)
	}
	if preview.VoucherCode != voucher.Code {
		t.Fatalf("expected voucher_code=%q, got %q", voucher.Code, preview.VoucherCode)
	}
	if preview.VoucherStatus != voucher.Status {
		t.Fatalf("expected voucher_status=%q, got %q", voucher.Status, preview.VoucherStatus)
	}
	if preview.CouponID != coupon.ID {
		t.Fatalf("expected coupon_id=%d, got %d", coupon.ID, preview.CouponID)
	}
	if preview.CouponTitle != coupon.Title {
		t.Fatalf("expected coupon_title=%q, got %q", coupon.Title, preview.CouponTitle)
	}
	if preview.StoreID == nil || *preview.StoreID != store.ID {
		t.Fatalf("expected store_id=%d, got %+v", store.ID, preview.StoreID)
	}
	if preview.StoreName != store.Name {
		t.Fatalf("expected store_name=%q, got %q", store.Name, preview.StoreName)
	}
	if preview.MerchantID != merchant.ID {
		t.Fatalf("expected merchant_id=%d, got %d", merchant.ID, preview.MerchantID)
	}
	if preview.MerchantName != merchant.Name {
		t.Fatalf("expected merchant_name=%q, got %q", merchant.Name, preview.MerchantName)
	}

	var refreshed model.Voucher
	if err := db.First(&refreshed, voucher.ID).Error; err != nil {
		t.Fatalf("failed to reload voucher: %v", err)
	}
	if refreshed.Status != "active" {
		t.Fatalf("expected preview to keep voucher active, got %q", refreshed.Status)
	}
	if refreshed.RedeemedAt != nil {
		t.Fatalf("expected preview to avoid setting redeemed_at, got %+v", refreshed.RedeemedAt)
	}
	if refreshed.RedeemedBy != nil {
		t.Fatalf("expected preview to avoid setting redeemed_by, got %+v", refreshed.RedeemedBy)
	}
}

func TestPreviewRedeemByTokenRejectsDifferentMerchant(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	issuingMerchantUserID := int64(1301)
	otherMerchantUserID := int64(1302)
	customerUserID := int64(1303)
	for _, id := range []int64{issuingMerchantUserID, otherMerchantUserID, customerUserID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}

	issuingMerchant := model.Merchant{Name: "Issuing Merchant", UserID: &issuingMerchantUserID}
	if err := db.Create(&issuingMerchant).Error; err != nil {
		t.Fatalf("failed to create issuing merchant: %v", err)
	}
	otherMerchant := model.Merchant{Name: "Other Merchant", UserID: &otherMerchantUserID}
	if err := db.Create(&otherMerchant).Error; err != nil {
		t.Fatalf("failed to create other merchant: %v", err)
	}
	store := model.Store{MerchantID: issuingMerchant.ID, Name: "Forbidden Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    issuingMerchant.ID,
		StoreID:       &storeID,
		Title:         "Forbidden Coupon",
		Type:          "cash",
		TotalQuantity: 100,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	voucher := model.Voucher{
		Code:       "voucher-preview-forbidden",
		ScanToken:  "scan-token-preview-forbidden",
		CouponID:   coupon.ID,
		UserID:     customerUserID,
		MerchantID: &issuingMerchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	_, err := svc.PreviewRedeemByToken(context.Background(), otherMerchantUserID, voucher.ScanToken)
	if err != ErrVoucherForbidden {
		t.Fatalf("expected ErrVoucherForbidden, got %v", err)
	}
}

func TestPreviewRedeemByTokenReturnsUsedVoucherAsNotRedeemable(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	merchantUserID := int64(1401)
	customerUserID := int64(1402)
	for _, id := range []int64{merchantUserID, customerUserID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}

	merchant := model.Merchant{Name: "Used Preview Merchant", UserID: &merchantUserID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Used Preview Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Used Preview Coupon",
		Type:          "cash",
		TotalQuantity: 100,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	redeemedAt := time.Now().Add(-1 * time.Hour)
	redeemedBy := merchantUserID
	voucher := model.Voucher{
		Code:       "voucher-preview-used",
		ScanToken:  "scan-token-preview-used",
		CouponID:   coupon.ID,
		UserID:     customerUserID,
		MerchantID: &merchant.ID,
		Status:     "used",
		RedeemedAt: &redeemedAt,
		RedeemedBy: &redeemedBy,
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	preview, err := svc.PreviewRedeemByToken(context.Background(), merchantUserID, voucher.ScanToken)
	if err != nil {
		t.Fatalf("preview redeem returned error: %v", err)
	}
	if preview.CanRedeem {
		t.Fatalf("expected used voucher to be non-redeemable")
	}
	if preview.Reason != "used" {
		t.Fatalf("expected reason used, got %q", preview.Reason)
	}
	if preview.RedeemedAt == nil {
		t.Fatalf("expected redeemed_at to be present for used voucher preview")
	}
}

func TestPreviewRedeemByTokenReturnsExpiredVoucherAsNotRedeemable(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	merchantUserID := int64(1501)
	customerUserID := int64(1502)
	for _, id := range []int64{merchantUserID, customerUserID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}

	merchant := model.Merchant{Name: "Expired Preview Merchant", UserID: &merchantUserID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Expired Preview Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Expired Preview Coupon",
		Type:          "cash",
		TotalQuantity: 100,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	expiredAt := time.Now().Add(-1 * time.Hour)
	voucher := model.Voucher{
		Code:       "voucher-preview-expired",
		ScanToken:  "scan-token-preview-expired",
		CouponID:   coupon.ID,
		UserID:     customerUserID,
		MerchantID: &merchant.ID,
		Status:     "active",
		ValidUntil: &expiredAt,
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	preview, err := svc.PreviewRedeemByToken(context.Background(), merchantUserID, voucher.ScanToken)
	if err != nil {
		t.Fatalf("preview redeem returned error: %v", err)
	}
	if preview.CanRedeem {
		t.Fatalf("expected expired voucher to be non-redeemable")
	}
	if preview.Reason != "expired" {
		t.Fatalf("expected reason expired, got %q", preview.Reason)
	}
}

func TestRedeemByTokenMarksVoucherUsed(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	merchantUserID := int64(1601)
	customerUserID := int64(1602)
	for _, id := range []int64{merchantUserID, customerUserID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}

	merchant := model.Merchant{Name: "Redeem Token Merchant", UserID: &merchantUserID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Redeem Token Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Redeem Token Coupon",
		Type:          "cash",
		TotalQuantity: 100,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	voucher := model.Voucher{
		Code:       "voucher-redeem-token",
		ScanToken:  "scan-token-redeem-success",
		CouponID:   coupon.ID,
		UserID:     customerUserID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	if err := svc.RedeemByMerchantToken(context.Background(), merchantUserID, voucher.ScanToken); err != nil {
		t.Fatalf("redeem by token returned error: %v", err)
	}

	var refreshedVoucher model.Voucher
	if err := db.First(&refreshedVoucher, voucher.ID).Error; err != nil {
		t.Fatalf("failed to reload voucher: %v", err)
	}
	if refreshedVoucher.Status != "used" {
		t.Fatalf("expected voucher status used, got %q", refreshedVoucher.Status)
	}
	if refreshedVoucher.RedeemedAt == nil {
		t.Fatalf("expected redeemed_at to be set")
	}
	if refreshedVoucher.RedeemedBy == nil || *refreshedVoucher.RedeemedBy != merchantUserID {
		t.Fatalf("expected redeemed_by=%d, got %+v", merchantUserID, refreshedVoucher.RedeemedBy)
	}

	var refreshedCoupon model.Coupon
	if err := db.Unscoped().First(&refreshedCoupon, coupon.ID).Error; err != nil {
		t.Fatalf("failed to reload coupon: %v", err)
	}
	if refreshedCoupon.RedeemedCount != 1 {
		t.Fatalf("expected redeemed_count=1, got %d", refreshedCoupon.RedeemedCount)
	}

	var audit model.OperationalAuditLog
	if err := db.Where("action = ? AND target_id = ? AND result = ?", "voucher.redeem_by_token", voucher.ID, "success").First(&audit).Error; err != nil {
		t.Fatalf("expected successful redemption audit: %v", err)
	}
}

func TestRedeemByTokenRejectsRepeatedRedemption(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	merchantUserID := int64(1701)
	customerUserID := int64(1702)
	for _, id := range []int64{merchantUserID, customerUserID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}

	merchant := model.Merchant{Name: "Repeated Redeem Merchant", UserID: &merchantUserID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Repeated Redeem Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Repeated Redeem Coupon",
		Type:          "cash",
		TotalQuantity: 100,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	voucher := model.Voucher{
		Code:       "voucher-redeem-repeat",
		ScanToken:  "scan-token-redeem-repeat",
		CouponID:   coupon.ID,
		UserID:     customerUserID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	if err := svc.RedeemByMerchantToken(context.Background(), merchantUserID, voucher.ScanToken); err != nil {
		t.Fatalf("first redeem returned error: %v", err)
	}

	err := svc.RedeemByMerchantToken(context.Background(), merchantUserID, voucher.ScanToken)
	if err != ErrVoucherNotRedeemable {
		t.Fatalf("expected ErrVoucherNotRedeemable, got %v", err)
	}

	var refreshedCoupon model.Coupon
	if err := db.Unscoped().First(&refreshedCoupon, coupon.ID).Error; err != nil {
		t.Fatalf("failed to reload coupon: %v", err)
	}
	if refreshedCoupon.RedeemedCount != 1 {
		t.Fatalf("expected redeemed_count to stay 1, got %d", refreshedCoupon.RedeemedCount)
	}
}

func TestRedeemByTokenRejectsWrongMerchant(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	issuingMerchantUserID := int64(1801)
	otherMerchantUserID := int64(1802)
	customerUserID := int64(1803)
	for _, id := range []int64{issuingMerchantUserID, otherMerchantUserID, customerUserID} {
		if err := db.Create(&model.User{ID: id, Role: "user", Status: 0}).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", id, err)
		}
	}

	issuingMerchant := model.Merchant{Name: "Issuing Redeem Merchant", UserID: &issuingMerchantUserID}
	if err := db.Create(&issuingMerchant).Error; err != nil {
		t.Fatalf("failed to create issuing merchant: %v", err)
	}
	otherMerchant := model.Merchant{Name: "Other Redeem Merchant", UserID: &otherMerchantUserID}
	if err := db.Create(&otherMerchant).Error; err != nil {
		t.Fatalf("failed to create other merchant: %v", err)
	}
	store := model.Store{MerchantID: issuingMerchant.ID, Name: "Wrong Merchant Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    issuingMerchant.ID,
		StoreID:       &storeID,
		Title:         "Wrong Merchant Coupon",
		Type:          "cash",
		TotalQuantity: 100,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	voucher := model.Voucher{
		Code:       "voucher-wrong-merchant",
		ScanToken:  "scan-token-wrong-merchant",
		CouponID:   coupon.ID,
		UserID:     customerUserID,
		MerchantID: &issuingMerchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	err := svc.RedeemByMerchantToken(context.Background(), otherMerchantUserID, voucher.ScanToken)
	if err != ErrVoucherForbidden {
		t.Fatalf("expected ErrVoucherForbidden, got %v", err)
	}
}

func TestMerchantCodePreviewAndRedeemFlow(t *testing.T) {
	db := setupVoucherTestDB(t)
	svc := NewVoucherService(db)

	merchantUser := model.User{ID: 1401, Role: "user", Status: 0}
	customerUser := model.User{ID: 1402, Role: "user", Status: 0}
	for _, user := range []model.User{merchantUser, customerUser} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("failed to create user %d: %v", user.ID, err)
		}
	}

	merchant := model.Merchant{Name: "Code Merchant", UserID: &merchantUser.ID}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}
	store := model.Store{MerchantID: merchant.ID, Name: "Code Store", Status: 1}
	if err := db.Create(&store).Error; err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	storeID := store.ID
	coupon := model.Coupon{
		MerchantID:    merchant.ID,
		StoreID:       &storeID,
		Title:         "Code Coupon",
		Type:          "discount",
		TotalQuantity: 10,
		MaxPerUser:    1,
		Status:        "active",
	}
	if err := db.Create(&coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	voucher := model.Voucher{
		Code:       "CODE-FLOW-VOUCHER",
		ScanToken:  "scan-token-code-flow",
		CouponID:   coupon.ID,
		UserID:     customerUser.ID,
		MerchantID: &merchant.ID,
		Status:     "active",
	}
	if err := db.Create(&voucher).Error; err != nil {
		t.Fatalf("failed to create voucher: %v", err)
	}

	preview, err := svc.PreviewRedeemByCode(context.Background(), merchantUser.ID, voucher.Code)
	if err != nil {
		t.Fatalf("preview by code returned error: %v", err)
	}
	if !preview.CanRedeem || preview.CouponTitle != coupon.Title {
		t.Fatalf("unexpected preview: %+v", preview)
	}

	if err := svc.RedeemByMerchantCode(context.Background(), merchantUser.ID, voucher.Code); err != nil {
		t.Fatalf("redeem by code returned error: %v", err)
	}

	var redeemed model.Voucher
	if err := db.First(&redeemed, voucher.ID).Error; err != nil {
		t.Fatalf("failed to reload voucher: %v", err)
	}
	if redeemed.Status != "used" {
		t.Fatalf("expected used status, got %q", redeemed.Status)
	}
	if redeemed.RedeemedBy == nil || *redeemed.RedeemedBy != merchantUser.ID {
		t.Fatalf("expected redeemed_by=%d, got %+v", merchantUser.ID, redeemed.RedeemedBy)
	}

	if err := svc.RedeemByMerchantCode(context.Background(), merchantUser.ID, voucher.Code); err != ErrVoucherNotRedeemable {
		t.Fatalf("expected repeated redemption to fail with ErrVoucherNotRedeemable, got %v", err)
	}
}
