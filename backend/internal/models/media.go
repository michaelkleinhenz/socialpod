package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MediaItem is an entry of the team's media library: an uploaded image,
// video or font with a name, a description of what it shows and tags, so
// people and agents can find material to build post images from. The file
// itself is an ordinary upload, referenced by URL. A colour entry has no file:
// it carries Color (#rrggbb or #rrggbbaa, ContentType MediaColorType) and its
// description says how to use it.
// MediaColorType is the content type of a colour entry.
const MediaColorType = "color/hex"

type MediaItem struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID      primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID      *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	URL         string              `bson:"url" json:"url"`
	Color       string              `bson:"color,omitempty" json:"color,omitempty"`
	Name        string              `bson:"name" json:"name"`
	Description string              `bson:"description,omitempty" json:"description,omitempty"`
	Tags        []string            `bson:"tags,omitempty" json:"tags,omitempty"`
	ContentType string              `bson:"contentType,omitempty" json:"contentType,omitempty"`
	Width       int                 `bson:"width,omitempty" json:"width,omitempty"`
	Height      int                 `bson:"height,omitempty" json:"height,omitempty"`
	Size        int64               `bson:"size,omitempty" json:"size,omitempty"`
	CreatedAt   time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time           `bson:"updatedAt" json:"updatedAt"`
}
