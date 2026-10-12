package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type NewsDraft struct {
	ID               primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID           primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID           *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	EpisodeNumber    string              `bson:"episodeNumber,omitempty" json:"episodeNumber,omitempty"`
	NewsTagline      string              `bson:"newsTagline,omitempty" json:"newsTagline,omitempty"`
	ArticleURL       string              `bson:"articleUrl,omitempty" json:"articleUrl,omitempty"`
	Shownotes        string              `bson:"shownotes,omitempty" json:"shownotes,omitempty"`
	ImageURLs        []string            `bson:"imageUrls,omitempty" json:"imageUrls,omitempty"`
	AddSocialPosting bool                `bson:"addSocialPosting" json:"addSocialPosting"`
	Content          string              `bson:"content,omitempty" json:"content,omitempty"`
	Platforms        []Platform          `bson:"platforms,omitempty" json:"platforms,omitempty"`
	ScheduledAt      string              `bson:"scheduledAt,omitempty" json:"scheduledAt,omitempty"`
	Tags             []string            `bson:"tags,omitempty" json:"tags,omitempty"`
	Status           PostStatus          `bson:"status,omitempty" json:"status,omitempty"`
	FooterIDs        map[string]string   `bson:"footerIds,omitempty" json:"footerIds,omitempty"`
	ContentOverrides map[string]string   `bson:"contentOverrides,omitempty" json:"contentOverrides,omitempty"`
	AccountIDs       map[string]string   `bson:"accountIds,omitempty" json:"accountIds,omitempty"`
	FirstComment     string              `bson:"firstComment,omitempty" json:"firstComment,omitempty"`
	PostType         PostType            `bson:"postType,omitempty" json:"postType,omitempty"`
	// Radar puts the draft in the Radar queue, from which the "Radar der
	// Woche" carousel is built. GermanRelease is the badge on its slide:
	// "ja", "nein" or "offen".
	Radar         bool                `bson:"radar,omitempty" json:"radar,omitempty"`
	GermanRelease string              `bson:"germanRelease,omitempty" json:"germanRelease,omitempty"`
	// RadarPostID is the post that carries this draft's Radar slide. While
	// that post has not gone out the draft stays in the Radar queue, marked as
	// scheduled; once it is published the draft was used and leaves the queue.
	RadarPostID   *primitive.ObjectID `bson:"radarPostId,omitempty" json:"radarPostId,omitempty"`
	Posted           bool                `bson:"posted" json:"posted"`
	PostedAt         *time.Time          `bson:"postedAt,omitempty" json:"postedAt,omitempty"`
	CreatedAt        time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt        time.Time           `bson:"updatedAt" json:"updatedAt"`
}
