package services

import (
	"context"
	"log"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Scheduler struct {
	DB       *database.MongoDB
	Bluesky  *BlueskyService
	Instagram *InstagramService
	stop     chan struct{}
}

func NewScheduler(db *database.MongoDB, uploadDir string) *Scheduler {
	return &Scheduler{
		DB:        db,
		Bluesky:   &BlueskyService{DB: db, UploadDir: uploadDir},
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

func (s *Scheduler) suffixContent(ctx context.Context, suffixIDStr string) string {
	if suffixIDStr == "" {
		return ""
	}
	oid, err := primitive.ObjectIDFromHex(suffixIDStr)
	if err != nil {
		return ""
	}
	var suffix models.Suffix
	if err := s.DB.Suffixes().FindOne(ctx, bson.M{"_id": oid}).Decode(&suffix); err != nil {
		return ""
	}
	return suffix.Content
}

func (s *Scheduler) publishPost(ctx context.Context, post models.Post) {
	var results []models.PostResult
	allSuccess := true

	bluskySuffix := ""
	instagramSuffix := ""
	if post.SuffixIDs != nil {
		bluskySuffix = s.suffixContent(ctx, post.SuffixIDs["bluesky"])
		instagramSuffix = s.suffixContent(ctx, post.SuffixIDs["instagram"])
	}

	applyContent := func(base, suffix string) string {
		if suffix == "" {
			return base
		}
		return base + "\n" + suffix
	}

	for _, platform := range post.Platforms {
		var result models.PostResult
		result.Platform = platform

		switch platform {
		case models.PlatformBluesky:
			accountID := ""
			if post.AccountIDs != nil {
				accountID = post.AccountIDs["bluesky"]
			}
			postURI, postCID, err := s.Bluesky.Post(ctx, applyContent(post.Content, bluskySuffix), post.ImageURLs, accountID)
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

			if post.PostType == models.PostTypeStory {
				// Stories: publish the first media file as a story
				if len(post.ImageURLs) == 0 {
					result.Success = false
					result.Error = "story requires at least one image or video"
					allSuccess = false
				} else {
					postID, err := s.Instagram.PostStory(ctx, post.ImageURLs[0], accountID)
					if err != nil {
						result.Success = false
						result.Error = err.Error()
						allSuccess = false
						log.Printf("Instagram story failed: %v", err)
					} else {
						result.Success = true
						result.PostID = postID
						result.PostedAt = time.Now()
					}
				}
			} else if post.PostType == models.PostTypeReel {
				// Reels: publish the first media file as a reel with optional caption
				if len(post.ImageURLs) == 0 {
					result.Success = false
					result.Error = "reel requires a video"
					allSuccess = false
				} else {
					postID, err := s.Instagram.PostReel(ctx, post.ImageURLs[0], applyContent(post.Content, instagramSuffix), accountID)
					if err != nil {
						result.Success = false
						result.Error = err.Error()
						allSuccess = false
						log.Printf("Instagram reel failed: %v", err)
					} else {
						result.Success = true
						result.PostID = postID
						result.PostedAt = time.Now()
					}
				}
			} else {
				postID, err := s.Instagram.Post(ctx, applyContent(post.Content, instagramSuffix), post.ImageURLs, accountID)
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
