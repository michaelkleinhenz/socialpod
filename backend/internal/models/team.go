package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AvailablePlugins is the catalog of all optional plugin IDs that can be
// enabled per team by a global admin.
var AvailablePlugins = []string{
	"episode_news",
	"news_creator",
	"episode_creator",
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
	NewsCreatorURL          string              `bson:"newsCreatorUrl,omitempty" json:"newsCreatorUrl,omitempty"`
	NewsCreatorBearerToken  string              `bson:"newsCreatorBearerToken,omitempty" json:"-"`
	NewsCreatorWatermarkID     *primitive.ObjectID `bson:"newsCreatorWatermarkId,omitempty" json:"newsCreatorWatermarkId,omitempty"`
	EpisodeCreatorURL          string              `bson:"episodeCreatorUrl,omitempty" json:"episodeCreatorUrl,omitempty"`
	EpisodeCreatorBearerToken  string              `bson:"episodeCreatorBearerToken,omitempty" json:"-"`
	EpisodeCreatorWatermarkID  *primitive.ObjectID `bson:"episodeCreatorWatermarkId,omitempty" json:"episodeCreatorWatermarkId,omitempty"`
	BGGHandleLookupEnabled     *bool               `bson:"bggHandleLookupEnabled,omitempty" json:"bggHandleLookupEnabled,omitempty"`
	EnabledPlugins          []string            `bson:"enabledPlugins,omitempty" json:"enabledPlugins,omitempty"`
	CreatedAt               time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt               time.Time           `bson:"updatedAt" json:"updatedAt"`
}
