package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MentionEntry struct {
	ID        primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID    *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	Name      string              `bson:"name" json:"name"`
	Handles   map[string]string   `bson:"handles" json:"handles"` // platform -> @handle
	CreatedAt time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time           `bson:"updatedAt" json:"updatedAt"`
}
