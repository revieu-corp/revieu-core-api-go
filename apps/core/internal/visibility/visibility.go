package visibility

import (
	"context"
	"errors"
	"fmt"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/model"
	"gorm.io/gorm"
)

var ErrPrivateContent = errors.New("content is private")

// CanViewUserContent applies the account privacy policy to content owned by a
// user. Missing privacy rows retain the model's public-by-default behavior.
func CanViewUserContent(ctx context.Context, db *gorm.DB, ownerID, viewerID int64) (bool, error) {
	if ownerID == viewerID {
		return true, nil
	}

	var privacy model.UserPrivacy
	err := db.WithContext(ctx).First(&privacy, "user_id = ?", ownerID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && privacy.IsPublic) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if viewerID == 0 {
		return false, nil
	}

	var follow model.UserFollow
	err = db.WithContext(ctx).
		Where("follower_id = ? AND following_id = ?", viewerID, ownerID).
		First(&follow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ScopePublicContent limits a query to content visible to the current viewer.
// ownerColumn is an internal, fixed SQL identifier such as "posts.user_id";
// callers must not pass user input.
func ScopePublicContent(q *gorm.DB, ownerColumn string, viewerID int64) *gorm.DB {
	condition := fmt.Sprintf(`(
		NOT EXISTS (
			SELECT 1 FROM user_privacies privacy
			WHERE privacy.user_id = %s
		)
		OR EXISTS (
			SELECT 1 FROM user_privacies privacy
			WHERE privacy.user_id = %s AND privacy.is_public = ?
		)
		OR %s = ?
		OR EXISTS (
			SELECT 1 FROM user_follows follow
			WHERE follow.following_id = %s AND follow.follower_id = ?
		)
	)`, ownerColumn, ownerColumn, ownerColumn, ownerColumn)
	return q.Where(condition, true, viewerID, viewerID)
}
