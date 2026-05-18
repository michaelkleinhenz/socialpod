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
	DB        *database.MongoDB
	Bluesky   *BlueskyService
	Instagram *InstagramService
	Twitter   *TwitterService
	Mastodon  *MastodonService
	Threads   *ThreadsService
	LinkedIn  *LinkedInService
	stop      chan struct{}
}

func NewScheduler(db *database.MongoDB, uploadDir string) *Scheduler {
	return &Scheduler{
		DB:        db,
		Bluesky:   &BlueskyService{DB: db, UploadDir: uploadDir},
		Instagram: &InstagramService{DB: db},
		Twitter:   &TwitterService{DB: db, UploadDir: uploadDir},
		Mastodon:  &MastodonService{DB: db, UploadDir: uploadDir},
		Threads:   &ThreadsService{DB: db},
		LinkedIn:  &LinkedInService{DB: db, UploadDir: uploadDir},
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
	twitterSuffix := ""
	mastodonSuffix := ""
	threadsSuffix := ""
	linkedInSuffix := ""
	if post.SuffixIDs != nil {
		bluskySuffix = s.suffixContent(ctx, post.SuffixIDs["bluesky"])
		instagramSuffix = s.suffixContent(ctx, post.SuffixIDs["instagram"])
		twitterSuffix = s.suffixContent(ctx, post.SuffixIDs["twitter"])
		mastodonSuffix = s.suffixContent(ctx, post.SuffixIDs["mastodon"])
		threadsSuffix = s.suffixContent(ctx, post.SuffixIDs["threads"])
		linkedInSuffix = s.suffixContent(ctx, post.SuffixIDs["linkedin"])
	}

	platformContent := func(platform models.Platform) string {
		if post.ContentOverrides != nil {
			if override, ok := post.ContentOverrides[string(platform)]; ok && override != "" {
				return override
			}
		}
		return post.Content
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
			postURI, postCID, err := s.Bluesky.Post(ctx, applyContent(platformContent(models.PlatformBluesky), bluskySuffix), post.ImageURLs, accountID)
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

		case models.PlatformTwitter:
			accountID := ""
			if post.AccountIDs != nil {
				accountID = post.AccountIDs["twitter"]
			}
			tweetID, err := s.Twitter.Post(ctx, applyContent(platformContent(models.PlatformTwitter), twitterSuffix), post.ImageURLs, accountID)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				allSuccess = false
				log.Printf("Twitter post failed: %v", err)
			} else {
				result.Success = true
				result.PostID = tweetID
				result.PostedAt = time.Now()
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
					postID, err := s.Instagram.PostReel(ctx, post.ImageURLs[0], applyContent(platformContent(models.PlatformInstagram), instagramSuffix), accountID)
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
				postID, err := s.Instagram.Post(ctx, applyContent(platformContent(models.PlatformInstagram), instagramSuffix), post.ImageURLs, accountID)
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
		case models.PlatformMastodon:
			accountID := ""
			if post.AccountIDs != nil {
				accountID = post.AccountIDs["mastodon"]
			}
			statusURL, err := s.Mastodon.Post(ctx, applyContent(platformContent(models.PlatformMastodon), mastodonSuffix), post.ImageURLs, accountID)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				allSuccess = false
				log.Printf("Mastodon post failed: %v", err)
			} else {
				result.Success = true
				result.PostID = statusURL
				result.PostedAt = time.Now()
			}

		case models.PlatformThreads:
			accountID := ""
			if post.AccountIDs != nil {
				accountID = post.AccountIDs["threads"]
			}
			postID, err := s.Threads.Post(ctx, applyContent(platformContent(models.PlatformThreads), threadsSuffix), post.ImageURLs, accountID)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				allSuccess = false
				log.Printf("Threads post failed: %v", err)
			} else {
				result.Success = true
				result.PostID = postID
				result.PostedAt = time.Now()
			}

		case models.PlatformLinkedIn:
			accountID := ""
			if post.AccountIDs != nil {
				accountID = post.AccountIDs["linkedin"]
			}
			postID, err := s.LinkedIn.Post(ctx, applyContent(platformContent(models.PlatformLinkedIn), linkedInSuffix), post.ImageURLs, accountID)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				allSuccess = false
				log.Printf("LinkedIn post failed: %v", err)
			} else {
				result.Success = true
				result.PostID = postID
				result.PostedAt = time.Now()
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
