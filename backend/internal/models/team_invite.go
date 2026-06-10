package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TeamInvite struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TeamID    primitive.ObjectID `bson:"teamId" json:"teamId"`
	Email     string             `bson:"email" json:"email"`
	TokenHash string             `bson:"tokenHash" json:"-"`
	Status    string             `bson:"status" json:"status"`
	InvitedBy primitive.ObjectID `bson:"invitedBy" json:"invitedBy"`
	ExpiresAt time.Time          `bson:"expiresAt" json:"expiresAt"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}
