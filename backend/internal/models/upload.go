package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Upload struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Filename    string             `bson:"filename" json:"filename"`
	ContentType string             `bson:"contentType" json:"contentType"`
	Data        []byte             `bson:"data" json:"-"`
	Size        int64              `bson:"size" json:"size"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
}
