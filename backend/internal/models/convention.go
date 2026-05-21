package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	ConventionQueueItemStatusPending   = "pending"
	ConventionQueueItemStatusApproved  = "approved"
	ConventionQueueItemStatusScheduled = "scheduled"
	ConventionQueueItemStatusPublished = "published"

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
	TimeSlots     []string            `bson:"timeSlots" json:"timeSlots"`
	Platforms     []Platform          `bson:"platforms" json:"platforms"`
	AccountIDs    map[string]string   `bson:"accountIds,omitempty" json:"accountIds,omitempty"`
	SuffixIDs     map[string]string   `bson:"suffixIds,omitempty" json:"suffixIds,omitempty"`
	Status        string              `bson:"status" json:"status"`
	CreatedAt     time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time           `bson:"updatedAt" json:"updatedAt"`
}

type ConventionQueueItem struct {
	ID         primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	QueueID    primitive.ObjectID  `bson:"queueId" json:"queueId"`
	PostID     *primitive.ObjectID `bson:"postId,omitempty" json:"postId,omitempty"`
	ImageURL   string              `bson:"imageUrl" json:"imageUrl"`
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
