package handlers

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Quick Reel: on a convention, a clip goes into the queue's reel template
// (rendered in the browser like the image editor's video export), gets an
// Instagram caption written from a few words of the user's, and is scheduled
// as an Instagram reel half an hour after the clip was uploaded.

// quickReelDelay is how long after the clip's upload the reel goes out.
const quickReelDelay = 30 * time.Minute

// quickReelOrigin marks the posts Quick Reel creates. Its drafts are a
// person's, not an agent's, so the agent draft queue leaves them out.
const quickReelOrigin = "quick-reel"

// quickReelHintsMax caps the hints sent to the model.
const quickReelHintsMax = 500

// loadQuickReelQueue loads the caller's queue named in the path.
func (h *ConventionHandler) loadQuickReelQueue(ctx context.Context, c *gin.Context) (models.ConventionQueue, bool) {
	var queue models.ConventionQueue
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return queue, false
	}
	filter := conventionQueueFilter(c)
	filter["_id"] = queueID
	if err := h.DB.ConventionQueues().FindOne(ctx, filter).Decode(&queue); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
		return queue, false
	}
	return queue, true
}

// quickReelCaptionInput is what the model gets: the convention and the
// user's words.
func quickReelCaptionInput(queue models.ConventionQueue, hints string) string {
	lines := []string{
		"Write the Instagram caption for a short reel filmed at a board game convention.",
		fmt.Sprintf("Convention: %s", queue.Name),
	}
	if queue.ConventionURL != "" {
		lines = append(lines, fmt.Sprintf("Convention website: %s", queue.ConventionURL))
	}
	lines = append(lines, fmt.Sprintf("What the reel shows, in the words of the person who filmed it (games, publishers, people, impressions): %s", hints))
	if len(queue.Hashtags) > 0 {
		lines = append(lines, fmt.Sprintf("Use these convention hashtags among the 3 to 5 hashtags: %s", strings.Join(queue.Hashtags, " ")))
	}
	lines = append(lines, "Only state what these words say; do not invent details about the games or the booth.")
	return strings.Join(lines, "\n")
}

// QuickReelCaption writes an Instagram caption for a Quick Reel from the
// user's hints, with the team's "Rewrite for Instagram" prompt (the growth
// plan's caption rules: hook, share prompt, closing question, 3-5 hashtags).
// POST /api/convention/queues/:id/reel/caption {"hints": "..."}
func (h *ConventionHandler) QuickReelCaption(c *gin.Context) {
	var input struct {
		Hints string `json:"hints"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Hints) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a few words about the clip first"})
		return
	}
	hints := strings.TrimSpace(input.Hints)
	if r := []rune(hints); len(r) > quickReelHintsMax {
		hints = string(r[:quickReelHintsMax])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	queue, ok := h.loadQuickReelQueue(ctx, c)
	if !ok {
		return
	}

	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil || settings.OpenRouterAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OpenRouter is not configured"})
		return
	}

	system := DefaultInstagramRewritePrompt
	if teamID := teamIDFrom(c); teamID != nil {
		var team models.Team
		if h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team) == nil && team.Growth != nil && strings.TrimSpace(team.Growth.InstagramRewritePrompt) != "" {
			system = team.Growth.InstagramRewritePrompt
		}
	}
	system += " There is no original post here: write the caption from the notes you are given."
	system += fmt.Sprintf(" The caption must fit within %d characters.", instagramCharLimit)
	system += "\n\n" + gameNameQuotingRule
	if settings.AILanguage != "" {
		system += fmt.Sprintf(" Write in %s.", settings.AILanguage)
	}

	text, err := openRouterChat(ctx, settings, system, quickReelCaptionInput(queue, hints))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"caption": text})
}

// quickReelTime is when a Quick Reel goes out: half an hour after its clip
// was uploaded, or right away when that is already past. An upload time that
// is missing or in the future counts as now.
func quickReelTime(uploadedAt, now time.Time) time.Time {
	if uploadedAt.IsZero() || uploadedAt.After(now) {
		uploadedAt = now
	}
	at := uploadedAt.Add(quickReelDelay)
	if at.Before(now) {
		return now
	}
	return at
}

// PublishQuickReel stores the rendered reel and schedules it as an Instagram
// reel on the queue's Instagram account and footer, or with draft=true keeps
// it as a draft (at the same suggested time) to finish in the PostEditor.
// POST /api/convention/queues/:id/reel (multipart: video, caption, uploadedAt, draft)
func (h *ConventionHandler) PublishQuickReel(c *gin.Context) {
	_, fh, err := c.Request.FormFile("video")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No video provided"})
		return
	}
	if ext := strings.ToLower(filepath.Ext(fh.Filename)); ext != ".mp4" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The reel must be an MP4"})
		return
	}
	draft := c.PostForm("draft") == "true"
	caption := strings.TrimSpace(c.PostForm("caption"))
	// Drafts are not checked, like in the editor: they may be unfinished.
	if caption == "" && !draft {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The reel needs a caption"})
		return
	}
	if n := len([]rune(caption)); n > instagramCharLimit && !draft {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("The caption is %d characters long, Instagram allows %d", n, instagramCharLimit)})
		return
	}
	var uploadedAt time.Time
	if s := c.PostForm("uploadedAt"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			uploadedAt = t
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	queue, ok := h.loadQuickReelQueue(ctx, c)
	if !ok {
		return
	}

	ph := &PostHandler{DB: h.DB, UploadDir: h.UploadDir}
	videoURL, err := ph.saveUpload(ctx, fh)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	now := time.Now()
	status := models.PostStatusScheduled
	if draft {
		status = models.PostStatusDraft
	}
	post := models.Post{
		UserID:      uid,
		TeamID:      queue.TeamID,
		PostType:    models.PostTypeReel,
		Content:     caption,
		ImageURLs:   []string{videoURL},
		Platforms:   []models.Platform{models.PlatformInstagram},
		ScheduledAt: quickReelTime(uploadedAt, now),
		Status:      status,
		Origin:      quickReelOrigin,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if id := queue.AccountIDs[string(models.PlatformInstagram)]; id != "" {
		post.AccountIDs = map[string]string{string(models.PlatformInstagram): id}
	}
	if id := queue.FooterIDs[string(models.PlatformInstagram)]; id != "" {
		post.FooterIDs = map[string]string{string(models.PlatformInstagram): id}
	}

	res, err := h.DB.Posts().InsertOne(ctx, post)
	if err != nil {
		cleanupUploads(h.DB, h.UploadDir, []string{videoURL})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save the reel"})
		return
	}
	post.ID = res.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, post)
}
