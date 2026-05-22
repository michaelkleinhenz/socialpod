package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Team struct {
	ID              primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Name            string              `bson:"name" json:"name"`
	APIToken        string              `bson:"apiToken,omitempty" json:"apiToken,omitempty"`
	BGGWatermarkID  *primitive.ObjectID `bson:"bggWatermarkId,omitempty" json:"bggWatermarkId,omitempty"`
	BGGCoverOffsetX int                 `bson:"bggCoverOffsetX,omitempty" json:"bggCoverOffsetX,omitempty"`
	BGGCoverOffsetY int                 `bson:"bggCoverOffsetY,omitempty" json:"bggCoverOffsetY,omitempty"`
	CreatedAt       time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time           `bson:"updatedAt" json:"updatedAt"`
}
