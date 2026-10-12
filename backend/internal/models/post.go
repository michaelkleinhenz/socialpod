package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type PostStatus string

const (
	PostStatusDraft     PostStatus = "draft"
	PostStatusScheduled PostStatus = "scheduled"
	PostStatusPublished PostStatus = "published"
	PostStatusFailed    PostStatus = "failed"
)

type Platform string

const (
	PlatformBluesky   Platform = "bluesky"
	PlatformInstagram Platform = "instagram"
	PlatformTwitter   Platform = "twitter"
	PlatformMastodon  Platform = "mastodon"
	PlatformThreads   Platform = "threads"
	PlatformLinkedIn  Platform = "linkedin"
	PlatformYouTube   Platform = "youtube"
)

type PostType string

const (
	PostTypePost  PostType = "post"
	PostTypeStory PostType = "story"
	PostTypeReel  PostType = "reel"
)

type PostResult struct {
	Platform  Platform  `bson:"platform" json:"platform"`
	Success   bool      `bson:"success" json:"success"`
	PostID    string    `bson:"postId,omitempty" json:"postId,omitempty"`
	PostCID   string    `bson:"postCid,omitempty" json:"postCid,omitempty"`
	Error     string    `bson:"error,omitempty" json:"error,omitempty"`
	PostedAt  time.Time `bson:"postedAt,omitempty" json:"postedAt,omitempty"`
	// Warning records something that went wrong without failing the post,
	// e.g. collaborators Instagram refused, so the post went out without them.
	Warning string `bson:"warning,omitempty" json:"warning,omitempty"`
	// Insights holds the platform's performance numbers for the published
	// media. Only Instagram fills it, refreshed by the scheduler.
	Insights *PostInsights `bson:"insights,omitempty" json:"insights,omitempty"`
}

// PostInsights are the Instagram Insights of one published media object.
// Metrics Instagram does not report for a media type (follows and profile
// visits on reels, for instance) stay zero.
type PostInsights struct {
	MediaProductType  string    `bson:"mediaProductType,omitempty" json:"mediaProductType,omitempty"`
	Permalink         string    `bson:"permalink,omitempty" json:"permalink,omitempty"`
	Reach             int       `bson:"reach" json:"reach"`
	Views             int       `bson:"views" json:"views"`
	Shares            int       `bson:"shares" json:"shares"`
	Saved             int       `bson:"saved" json:"saved"`
	Likes             int       `bson:"likes" json:"likes"`
	Comments          int       `bson:"comments" json:"comments"`
	TotalInteractions int       `bson:"totalInteractions" json:"totalInteractions"`
	Follows           int       `bson:"follows" json:"follows"`
	ProfileVisits     int       `bson:"profileVisits" json:"profileVisits"`
	FetchedAt         time.Time `bson:"fetchedAt" json:"fetchedAt"`
	Error             string    `bson:"error,omitempty" json:"error,omitempty"`
}

type NewsResult struct {
	Success bool      `bson:"success" json:"success"`
	Error   string    `bson:"error,omitempty" json:"error,omitempty"`
	SentAt  time.Time `bson:"sentAt,omitempty" json:"sentAt,omitempty"`
}

type EpisodeNews struct {
	Enabled        bool          `bson:"enabled" json:"enabled"`
	EpisodeNumber  string        `bson:"episodeNumber,omitempty" json:"episodeNumber,omitempty"`
	Title          string        `bson:"title,omitempty" json:"title,omitempty"`
	AdditionalText string        `bson:"additionalText,omitempty" json:"additionalText,omitempty"`
	BGGLink        string        `bson:"bggLink,omitempty" json:"bggLink,omitempty"`
	Result         *NewsResult   `bson:"result,omitempty" json:"result,omitempty"`
	ResultHistory  []NewsResult  `bson:"resultHistory,omitempty" json:"resultHistory,omitempty"`
}

type Post struct {
	ID           primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID       primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID       *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	PostType     PostType           `bson:"postType,omitempty" json:"postType,omitempty"`
	Content      string             `bson:"content" json:"content"`
	FirstComment string             `bson:"firstComment,omitempty" json:"firstComment,omitempty"`
	ImageURLs    []string           `bson:"imageUrls,omitempty" json:"imageUrls,omitempty"`
	Platforms    []Platform         `bson:"platforms" json:"platforms"`
	ScheduledAt  time.Time          `bson:"scheduledAt" json:"scheduledAt"`
	Status       PostStatus         `bson:"status" json:"status"`
	Results      []PostResult       `bson:"results,omitempty" json:"results,omitempty"`
	Tags         []string           `bson:"tags,omitempty" json:"tags,omitempty"`
	AccountIDs      map[string]string  `bson:"accountIds,omitempty" json:"accountIds,omitempty"`
	FooterIDs       map[string]string  `bson:"footerIds,omitempty" json:"footerIds,omitempty"`
	ContentOverrides map[string]string `bson:"contentOverrides,omitempty" json:"contentOverrides,omitempty"`
	EpisodeNews  *EpisodeNews       `bson:"episodeNews,omitempty" json:"episodeNews,omitempty"`
	// Collaborators are Instagram usernames invited as collaborators (max 3);
	// the post then appears on their profile too once they accept.
	Collaborators []string `bson:"collaborators,omitempty" json:"collaborators,omitempty"`
	// UserTags are Instagram usernames tagged on the image (on the first slide
	// of a carousel, on the video of a reel).
	UserTags []string `bson:"userTags,omitempty" json:"userTags,omitempty"`
	// RadarDraftIDs are the news drafts whose Radar slides this post carries.
	// Saving the post takes them out of the Radar queue.
	RadarDraftIDs []primitive.ObjectID `bson:"radarDraftIds,omitempty" json:"radarDraftIds,omitempty"`
	CreatedAt    time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time          `bson:"updatedAt" json:"updatedAt"`
}
