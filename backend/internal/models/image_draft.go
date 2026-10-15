package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ImageDraft is image editor work saved to be finished later: the whole edit
// as JSON (Design: the design as a template, and for an edited photo the photo,
// its crop and filters), a rendered preview (URL) and the uploads the design
// uses (DesignURLs: the photo and the image layers), so they stay while the
// draft does. A draft belongs to the user who saved it.
type ImageDraft struct {
	ID         primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID     primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID     *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	Name       string              `bson:"name" json:"name"`
	URL        string              `bson:"url" json:"url"`
	Design     string              `bson:"design" json:"design,omitempty"`
	DesignURLs []string            `bson:"designUrls,omitempty" json:"-"`
	Width      int                 `bson:"width,omitempty" json:"width,omitempty"`
	Height     int                 `bson:"height,omitempty" json:"height,omitempty"`
	CreatedAt  time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time           `bson:"updatedAt" json:"updatedAt"`
}
