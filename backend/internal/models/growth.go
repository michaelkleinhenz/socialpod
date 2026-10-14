package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TeamGrowthSettings holds a team's Instagram growth configuration: the
// follower goal, the weekly content plan and the Instagram caption tone. Team
// admins edit it; every member reads it.
type TeamGrowthSettings struct {
	InstagramGoal          *InstagramGoal   `bson:"instagramGoal,omitempty" json:"instagramGoal,omitempty"`
	WeeklyPlan             []WeeklyPlanSlot `bson:"weeklyPlan,omitempty" json:"weeklyPlan"`
	MaxSingleImagesPerWeek int              `bson:"maxSingleImagesPerWeek,omitempty" json:"maxSingleImagesPerWeek,omitempty"`
	InstagramRewritePrompt string           `bson:"instagramRewritePrompt,omitempty" json:"instagramRewritePrompt,omitempty"`
	// MaxAgentDrafts caps the open drafts MCP clients and agents may have
	// waiting for review at once (posts, news and episode drafts with an
	// origin); 0 means no limit.
	MaxAgentDrafts int `bson:"maxAgentDrafts,omitempty" json:"maxAgentDrafts,omitempty"`
}

// InstagramGoal is a follower target with a date range. Dates are
// "2006-01-02". StartFollowers overrides the count read from the first
// snapshot on or after StartDate.
type InstagramGoal struct {
	Target           int     `bson:"target" json:"target"`
	StartDate        string  `bson:"startDate" json:"startDate"`
	EndDate          string  `bson:"endDate" json:"endDate"`
	StartFollowers   *int    `bson:"startFollowers,omitempty" json:"startFollowers,omitempty"`
	HalfwayDate      string  `bson:"halfwayDate,omitempty" json:"halfwayDate,omitempty"`
	HalfwayThreshold float64 `bson:"halfwayThreshold,omitempty" json:"halfwayThreshold,omitempty"`
}

// Post formats a weekly plan slot can ask for.
const (
	SlotFormatReel     = "reel"
	SlotFormatCarousel = "carousel"
	SlotFormatImage    = "image"
	SlotFormatStory    = "story"
	SlotFormatAny      = "any"
)

// WeeklyPlanSlot is one recurring entry of the weekly content plan.
// Weekday is 0 for Monday through 6 for Sunday; Time is "15:04" or empty.
type WeeklyPlanSlot struct {
	ID      string `bson:"id" json:"id"`
	Weekday int    `bson:"weekday" json:"weekday"`
	Time    string `bson:"time,omitempty" json:"time,omitempty"`
	Format  string `bson:"format" json:"format"`
	Label   string `bson:"label,omitempty" json:"label,omitempty"`
}

// FollowerSnapshot is one day's follower count of an Instagram account. The
// scheduler upserts today's snapshot every hour, so a day's entry holds the
// last count read that day. Date is "2006-01-02" in UTC.
type FollowerSnapshot struct {
	ID             primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	AccountID      primitive.ObjectID  `bson:"accountId" json:"accountId"`
	TeamID         *primitive.ObjectID `bson:"teamId,omitempty" json:"teamId,omitempty"`
	Date           string              `bson:"date" json:"date"`
	FollowersCount int                 `bson:"followersCount" json:"followersCount"`
	FollowsCount   int                 `bson:"followsCount" json:"followsCount"`
	MediaCount     int                 `bson:"mediaCount" json:"mediaCount"`
	UpdatedAt      time.Time           `bson:"updatedAt" json:"updatedAt"`
}
