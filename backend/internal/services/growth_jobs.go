package services

import (
	"context"
	"log"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// insightsWindow is how far back published Instagram posts keep getting their
// Insights refreshed. Older numbers barely move.
const insightsWindow = 60 * 24 * time.Hour

func (s *Scheduler) runGrowthJobs() {
	s.SnapshotFollowers()
	s.refreshDueInsights()
}

// SnapshotFollowers stores today's follower count for every active Instagram
// account. It runs hourly and upserts, so each day keeps the last count read.
func (s *Scheduler) SnapshotFollowers() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cursor, err := s.DB.SocialAccounts().Find(ctx, bson.M{"platform": models.PlatformInstagram, "isActive": true})
	if err != nil {
		log.Printf("Follower snapshot: failed to list accounts: %v", err)
		return
	}
	var accounts []models.SocialAccount
	if err := cursor.All(ctx, &accounts); err != nil {
		log.Printf("Follower snapshot: failed to decode accounts: %v", err)
		return
	}

	for _, account := range accounts {
		if err := RecordFollowerSnapshot(ctx, s.DB, s.Instagram, &account); err != nil {
			log.Printf("Follower snapshot: %s: %v", account.AccountName, err)
		}
	}
}

// RecordFollowerSnapshot reads an account's counters and upserts today's
// snapshot.
func RecordFollowerSnapshot(ctx context.Context, db *database.MongoDB, ig *InstagramService, account *models.SocialAccount) error {
	stats, err := ig.FetchAccountStats(ctx, account)
	if err != nil {
		return err
	}
	_, err = db.FollowerSnapshots().UpdateOne(ctx,
		bson.M{"accountId": account.ID, "date": time.Now().UTC().Format("2006-01-02")},
		bson.M{"$set": bson.M{
			"teamId":         account.TeamID,
			"followersCount": stats.FollowersCount,
			"followsCount":   stats.FollowsCount,
			"mediaCount":     stats.MediaCount,
			"updatedAt":      time.Now(),
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

// insightsDue reports whether a published post's Insights should be read
// again: hourly-ish while a post is fresh and its numbers move, then less
// often as it ages.
func insightsDue(postedAt time.Time, ins *models.PostInsights, now time.Time) bool {
	if ins == nil {
		return true
	}
	age := now.Sub(postedAt)
	since := now.Sub(ins.FetchedAt)
	switch {
	case age < 48*time.Hour:
		return since >= 3*time.Hour
	case age < 14*24*time.Hour:
		return since >= 12*time.Hour
	default:
		return since >= 48*time.Hour
	}
}

// refreshDueInsights refreshes the Insights of recent Instagram posts whose
// numbers are due.
func (s *Scheduler) refreshDueInsights() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	now := time.Now()
	cursor, err := s.DB.Posts().Find(ctx, bson.M{
		"results": bson.M{"$elemMatch": bson.M{
			"platform": models.PlatformInstagram,
			"success":  true,
			"postId":   bson.M{"$nin": bson.A{"", nil}},
			"postedAt": bson.M{"$gte": now.Add(-insightsWindow)},
		}},
	})
	if err != nil {
		log.Printf("Insights refresh: failed to list posts: %v", err)
		return
	}
	var posts []models.Post
	if err := cursor.All(ctx, &posts); err != nil {
		log.Printf("Insights refresh: failed to decode posts: %v", err)
		return
	}

	refreshed := 0
	for _, post := range posts {
		for _, r := range post.Results {
			if r.Platform != models.PlatformInstagram || !r.Success || r.PostID == "" || !insightsDue(r.PostedAt, r.Insights, now) {
				continue
			}
			// A story's Insights vanish 24 hours after it was posted.
			if post.PostType == models.PostTypeStory && now.Sub(r.PostedAt) > 24*time.Hour {
				continue
			}
			if s.RefreshPostInsights(ctx, post) == nil {
				refreshed++
			}
			break
		}
	}
	if refreshed > 0 {
		log.Printf("Insights refresh: refreshed %d posts", refreshed)
	}
}

// RefreshPostInsights reads the Instagram Insights of a published post and
// stores them on its Instagram result. A failed read is stored too, so the
// UI can say why the numbers are missing, without discarding older numbers.
func (s *Scheduler) RefreshPostInsights(ctx context.Context, post models.Post) error {
	for _, r := range post.Results {
		if r.Platform != models.PlatformInstagram || !r.Success || r.PostID == "" {
			continue
		}
		accountID := ""
		if post.AccountIDs != nil {
			accountID = post.AccountIDs["instagram"]
		}
		if accountID == "" {
			accountID = s.resolveAccountID(ctx, models.PlatformInstagram, post.TeamID)
		}
		account, err := s.Instagram.getAccount(ctx, accountID)
		if err != nil {
			return err
		}

		ins, fetchErr := s.Instagram.FetchMediaInsights(ctx, account, r.PostID)
		if fetchErr != nil {
			ins = r.Insights
			if ins == nil {
				ins = &models.PostInsights{}
			}
			copied := *ins
			copied.Error = fetchErr.Error()
			copied.FetchedAt = time.Now()
			ins = &copied
		}

		_, err = s.DB.Posts().UpdateOne(ctx,
			bson.M{"_id": post.ID},
			bson.M{"$set": bson.M{"results.$[r].insights": ins}},
			options.Update().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{
				bson.M{"r.platform": models.PlatformInstagram, "r.postId": r.PostID},
			}}),
		)
		if err != nil {
			return err
		}
		return fetchErr
	}
	return nil
}

// RefreshPostInsightsByID loads a post and refreshes its Insights.
func (s *Scheduler) RefreshPostInsightsByID(ctx context.Context, postID primitive.ObjectID) error {
	var post models.Post
	if err := s.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post); err != nil {
		return err
	}
	return s.RefreshPostInsights(ctx, post)
}
