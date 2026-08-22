package model

import "time"

type Conversation struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Type      string    `gorm:"type:varchar(20);default:'direct'" json:"type"`
	Title     string    `gorm:"type:varchar(100)" json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Participants []ConversationParticipant `gorm:"foreignKey:ConversationID" json:"participants,omitempty"`
	Messages     []Message                 `gorm:"foreignKey:ConversationID" json:"messages,omitempty"`
}

func (c *Conversation) TableName() string { return "conversations" }

type ConversationParticipant struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ConversationID int64     `gorm:"not null;index" json:"conversation_id"`
	UserID         int64     `gorm:"not null;index" json:"user_id"`
	Role           string    `gorm:"type:varchar(20);default:'member'" json:"role"`
	IsMuted        bool      `gorm:"default:false" json:"is_muted"`
	IsPinned       bool      `gorm:"default:false" json:"is_pinned"`
	LastReadAt     time.Time `json:"last_read_at"`
	JoinedAt       time.Time `json:"joined_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (cp *ConversationParticipant) TableName() string { return "conversation_participants" }
