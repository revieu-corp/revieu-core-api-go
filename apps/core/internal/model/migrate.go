package model

// All returns every model that participates in schema creation. It is the
// single source of truth for AutoMigrate (tests) and the ddldump tool that
// generates a manual SQL migration. Keep in sync with the domain models.
func All() []interface{} {
	return []interface{}{
		// User system
		&User{},
		&UserAuth{},
		&UserProfile{},
		&EmailVerification{},
		&RefreshToken{},
		// Social
		&UserFollow{},
		&MerchantFollow{},
		&Like{},
		&Favorite{},
		// User settings
		&UserAddress{},
		&UserPrivacy{},
		&UserNotification{},
		&AccountDeletion{},
		// Merchant & Store
		&Merchant{},
		&Category{},
		&StoreCategory{},
		&Store{},
		&StoreHour{},
		&Dish{},
		// Tags
		&Tag{},
		// Content
		&Review{},
		&ReviewMedia{},
		&ReviewComment{},
		&Post{},
		&PostComment{},
		// Commerce
		&Package{},
		&Coupon{},
		&Order{},
		&Voucher{},
		&Payment{},
		// Media
		&MediaUpload{},
		// Messaging
		&Conversation{},
		&ConversationParticipant{},
		&Message{},
		// Merchant verification
		&MerchantVerification{},
		// Marketing & Analytics
		&MarketingPost{},
		&MerchantAnalytics{},
		// Notifications
		&Notification{},
		// Reports & Admin
		&Report{},
		&AdminAuditLog{},
		// Browsing History
		&BrowsingHistory{},
	}
}
