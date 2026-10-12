package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Watermark struct {
	ID     primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	Name   string              `bson:"name" json:"name"`
	// Description says what the watermark looks like and what it is for
	// ("logo, corner", "full frame for news covers"), so an agent composing
	// images can pick one without seeing it.
	Description string    `bson:"description,omitempty" json:"description,omitempty"`
	Filename    string    `bson:"filename" json:"filename"`
	URL         string    `bson:"url" json:"url"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
}
