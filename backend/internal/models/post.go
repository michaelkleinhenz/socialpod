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
	AccountIDs      map[string]string  `bson:"accountIds,omitempty" json:"accountIds,omitempty"`      // platform -> accountId
	SuffixIDs       map[string]string  `bson:"suffixIds,omitempty" json:"suffixIds,omitempty"`       // platform -> suffixId
	ContentOverrides map[string]string `bson:"contentOverrides,omitempty" json:"contentOverrides,omitempty"` // platform -> custom content
	CreatedAt    time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time          `bson:"updatedAt" json:"updatedAt"`
}
