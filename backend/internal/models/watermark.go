package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Watermark struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name" json:"name"`
	Filename  string             `bson:"filename" json:"filename"`
	URL       string             `bson:"url" json:"url"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}
