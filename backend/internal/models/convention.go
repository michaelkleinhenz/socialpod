package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	ConventionQueueItemStatusPending   = "pending"
	ConventionQueueItemStatusApproved  = "approved"
	ConventionQueueItemStatusScheduled = "scheduled"

	ConventionQueueStatusActive    = "active"
	ConventionQueueStatusCompleted = "completed"
)

type ConventionQueue struct {
	ID            primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID        primitive.ObjectID  `bson:"userId" json:"userId"`
	TeamID        *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	Name          string              `bson:"name" json:"name"`
	ConventionURL string              `bson:"conventionUrl,omitempty" json:"conventionUrl,omitempty"`
	Hashtags      []string            `bson:"hashtags,omitempty" json:"hashtags,omitempty"`
	StartDate     time.Time           `bson:"startDate" json:"startDate"`
	EndDate       time.Time           `bson:"endDate" json:"endDate"`
	PostsPerDay   int                 `bson:"postsPerDay" json:"postsPerDay"`
	// MinHoursBetweenPosts is the minimum delay between two consecutive posts.
	// It takes precedence over PostsPerDay: it caps how many posts can go out
	// per day (floor(24 / MinHoursBetweenPosts)).
	MinHoursBetweenPosts float64 `bson:"minHoursBetweenPosts,omitempty" json:"minHoursBetweenPosts,omitempty"`
	// TimeSlots is retained for backward compatibility with existing queues.
	// Posts are now scattered randomly across each day rather than pinned to
	// fixed time slots, so this field is no longer used for scheduling.
	TimeSlots  []string          `bson:"timeSlots,omitempty" json:"timeSlots,omitempty"`
	Platforms  []Platform        `bson:"platforms" json:"platforms"`
	AccountIDs map[string]string `bson:"accountIds,omitempty" json:"accountIds,omitempty"`
	SuffixIDs  map[string]string `bson:"suffixIds,omitempty" json:"suffixIds,omitempty"`
	// WatermarkID, when set, is an overlay watermark that is composited onto
	// each image at post time.
	WatermarkID *primitive.ObjectID `bson:"watermarkId,omitempty" json:"watermarkId,omitempty"`
	Status      string              `bson:"status" json:"status"`
	// NextPostAt is the time the next random post from this queue is due. The
	// auto-poster picks one approved item at random whenever this time passes,
	// publishes it, and rolls NextPostAt forward by the schedule gap. A nil
	// value means the queue has not posted yet and is due as soon as it enters
	// its date window with at least one approved item.
	NextPostAt *time.Time `bson:"nextPostAt,omitempty" json:"nextPostAt,omitempty"`
	// PostedCount is the running total of items from this queue that have been
	// successfully published. Items are removed from the queue once published, so
	// this counter is the only record of how many have gone out.
	PostedCount int       `bson:"postedCount,omitempty" json:"postedCount,omitempty"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt" json:"updatedAt"`
}

type ConventionQueueItem struct {
	ID         primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	QueueID    primitive.ObjectID  `bson:"queueId" json:"queueId"`
	PostID     *primitive.ObjectID `bson:"postId,omitempty" json:"postId,omitempty"`
	ImageURL   string              `bson:"imageUrl" json:"imageUrl"`
	// ImageURLs holds every image for a gallery item (a single post with multiple
	// images). When empty the item is a single-image post backed by ImageURL,
	// which for a gallery item mirrors the first entry here for thumbnails and AI
	// analysis.
	ImageURLs  []string            `bson:"imageUrls,omitempty" json:"imageUrls,omitempty"`
	BGGURL     string              `bson:"bggUrl,omitempty" json:"bggUrl,omitempty"`
	Caption    string              `bson:"caption,omitempty" json:"caption,omitempty"`
	Status     string              `bson:"status" json:"status"`
	AIError    string              `bson:"aiError,omitempty" json:"aiError,omitempty"`
	SortOrder  int                 `bson:"sortOrder" json:"sortOrder"`
	Platforms  []Platform          `bson:"platforms,omitempty" json:"platforms,omitempty"`
	AccountIDs map[string]string   `bson:"accountIds,omitempty" json:"accountIds,omitempty"`
	SuffixIDs  map[string]string   `bson:"suffixIds,omitempty" json:"suffixIds,omitempty"`
	CreatedAt  time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time           `bson:"updatedAt" json:"updatedAt"`
}
