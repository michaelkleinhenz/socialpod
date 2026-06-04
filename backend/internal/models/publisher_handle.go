package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PublisherHandle maps a BGG name (publisher, designer, artist) to social-media
// handles across platforms. The catalog is global (admin-managed) and shared by
// all teams.
type PublisherHandle struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name" json:"name"`
	Handles   map[string]string  `bson:"handles" json:"handles"` // platform → @handle
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}
