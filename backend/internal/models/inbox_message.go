package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MessageType string

const (
	MessageTypeComment MessageType = "comment"
	MessageTypeDM      MessageType = "dm"
)

// InboxMessage stores a received comment or DM from a social platform.
type InboxMessage struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Platform     Platform           `bson:"platform" json:"platform"`
	MessageType  MessageType        `bson:"messageType" json:"messageType"`
	AccountID    primitive.ObjectID `bson:"accountId,omitempty" json:"accountId,omitempty"`
	AccountName  string             `bson:"accountName" json:"accountName"`
	ExternalID   string             `bson:"externalId" json:"externalId"` // platform-native ID (comment/message ID)
	ThreadID     string             `bson:"threadId,omitempty" json:"threadId,omitempty"`
	SenderID     string             `bson:"senderId" json:"senderId"`
	SenderName   string             `bson:"senderName,omitempty" json:"senderName,omitempty"`
	SenderAvatar string             `bson:"senderAvatar,omitempty" json:"senderAvatar,omitempty"`
	Text         string             `bson:"text" json:"text"`
	MediaID      string             `bson:"mediaId,omitempty" json:"mediaId,omitempty"` // for comments: which post
	MediaURL     string             `bson:"mediaUrl,omitempty" json:"mediaUrl,omitempty"`
	IsRead       bool               `bson:"isRead" json:"isRead"`
	IsReplied    bool               `bson:"isReplied" json:"isReplied"`
	ReceivedAt   time.Time          `bson:"receivedAt" json:"receivedAt"`
	CreatedAt    time.Time          `bson:"createdAt" json:"createdAt"`
}
