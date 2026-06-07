package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AvailableFeatures is the catalog of all optional feature IDs that can be
// enabled per team by a global admin.
var AvailableFeatures = []string{
	"episode_news",
}

type Team struct {
	ID                      primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Name                    string              `bson:"name" json:"name"`
	APIToken                string              `bson:"apiToken,omitempty" json:"apiToken,omitempty"`
	BGGWatermarkID          *primitive.ObjectID `bson:"bggWatermarkId,omitempty" json:"bggWatermarkId,omitempty"`
	BGGCoverOffsetX         int                 `bson:"bggCoverOffsetX,omitempty" json:"bggCoverOffsetX,omitempty"`
	BGGCoverOffsetY         int                 `bson:"bggCoverOffsetY,omitempty" json:"bggCoverOffsetY,omitempty"`
	EpisodeNewsURL          string              `bson:"episodeNewsUrl,omitempty" json:"episodeNewsUrl,omitempty"`
	EpisodeNewsBearerToken  string              `bson:"episodeNewsBearerToken,omitempty" json:"-"`
	BGGHandleLookupEnabled  *bool               `bson:"bggHandleLookupEnabled,omitempty" json:"bggHandleLookupEnabled,omitempty"`
	EnabledFeatures         []string            `bson:"enabledFeatures,omitempty" json:"enabledFeatures,omitempty"`
	CreatedAt               time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt               time.Time           `bson:"updatedAt" json:"updatedAt"`
}
