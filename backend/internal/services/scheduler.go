package services

import (
	"context"
	"log"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

type Scheduler struct {
	DB       *database.MongoDB
	Bluesky  *BlueskyService
	Instagram *InstagramService
	stop     chan struct{}
}

func NewScheduler(db *database.MongoDB) *Scheduler {
	return &Scheduler{
		DB:        db,
		Bluesky:   &BlueskyService{DB: db},
		Instagram: &InstagramService{DB: db},
		stop:      make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	log.Println("Post scheduler started")
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.processScheduledPosts()
			case <-s.stop:
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	close(s.stop)
}

func (s *Scheduler) processScheduledPosts() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find posts that are due
	cursor, err := s.DB.Posts().Find(ctx, bson.M{
		"status":      models.PostStatusScheduled,
		"scheduledAt": bson.M{"$lte": time.Now()},
	})
	if err != nil {
		log.Printf("Scheduler error: %v", err)
		return
	}
	defer cursor.Close(ctx)

	var posts []models.Post
	if err := cursor.All(ctx, &posts); err != nil {
		return
	}

	for _, post := range posts {
		s.publishPost(ctx, post)
	}
}

func (s *Scheduler) publishPost(ctx context.Context, post models.Post) {
	var results []models.PostResult
	allSuccess := true

	for _, platform := range post.Platforms {
		var result models.PostResult
		result.Platform = platform

		switch platform {
		case models.PlatformBluesky:
			accountID := ""
			if post.AccountIDs != nil {
				accountID = post.AccountIDs["bluesky"]
			}
			postURI, postCID, err := s.Bluesky.Post(ctx, post.Content, post.ImageURLs, accountID)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				allSuccess = false
				log.Printf("Bluesky post failed: %v", err)
			} else {
				result.Success = true
				result.PostID = postURI
				result.PostCID = postCID
				result.PostedAt = time.Now()
				if post.FirstComment != "" {
					if _, replyErr := s.Bluesky.PostReply(ctx, post.FirstComment, postURI, postCID, accountID); replyErr != nil {
						log.Printf("Bluesky first comment failed: %v", replyErr)
					}
				}
			}

		case models.PlatformInstagram:
			accountID := ""
			if post.AccountIDs != nil {
				accountID = post.AccountIDs["instagram"]
			}
			postID, err := s.Instagram.Post(ctx, post.Content, post.ImageURLs, accountID)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				allSuccess = false
				log.Printf("Instagram post failed: %v", err)
			} else {
				result.Success = true
				result.PostID = postID
				result.PostedAt = time.Now()
				if post.FirstComment != "" {
					if commentErr := s.Instagram.PostComment(ctx, post.FirstComment, postID, accountID); commentErr != nil {
						log.Printf("Instagram first comment failed: %v", commentErr)
					}
				}
			}
		}

		results = append(results, result)
	}

	status := models.PostStatusPublished
	if !allSuccess {
		status = models.PostStatusFailed
	}

	s.DB.Posts().UpdateOne(ctx, bson.M{"_id": post.ID}, bson.M{
		"$set": bson.M{
			"status":    status,
			"results":   results,
			"updatedAt": time.Now(),
		},
	})
}
