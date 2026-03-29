package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Suffix struct {
	ID        primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID    *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	Name      string              `bson:"name" json:"name"`
	Content   string              `bson:"content" json:"content"`
	CreatedAt time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time           `bson:"updatedAt" json:"updatedAt"`
}
