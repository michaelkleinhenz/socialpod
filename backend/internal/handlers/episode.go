package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type EpisodeHandler struct {
	DB        *database.MongoDB
	UploadDir string
}

type EpisodeSubmitInput struct {
	EpisodeNumber    string `json:"episodeNumber"`
	EpisodeTitle     string `json:"episodeTitle"`
	EpisodeType      string `json:"episodeType"`
	Summary          string `json:"summary,omitempty"`
	EpisodeDate      string `json:"episodeDate"`

	// Review-specific fields
	GameNamePublisher string `json:"gameNamePublisher,omitempty"`
	LinkPublisher     string `json:"linkPublisher,omitempty"`
	LinkBGG           string `json:"linkBGG,omitempty"`
	Rules             string `json:"rules,omitempty"`
	Scene             string `json:"scene,omitempty"`
	IntroText         string `json:"introText,omitempty"`

	// Social posting
	AddSocialPosting bool              `json:"addSocialPosting"`
	Content          string            `json:"content,omitempty"`
	Platforms        []models.Platform `json:"platforms,omitempty"`
	ScheduledAt      string            `json:"scheduledAt,omitempty"`
	ImageURLs        []string          `json:"imageUrls,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	Status           models.PostStatus `json:"status,omitempty"`
	FooterIDs        map[string]string `json:"footerIds,omitempty"`
	ContentOverrides map[string]string `json:"contentOverrides,omitempty"`
	AccountIDs       map[string]string `json:"accountIds,omitempty"`
	FirstComment     string            `json:"firstComment,omitempty"`
	PostType         models.PostType   `json:"postType,omitempty"`
	DraftID          string            `json:"draftId,omitempty"`
}

func (h *EpisodeHandler) Submit(c *gin.Context) {
	var input EpisodeSubmitInput
	if err := json.Unmarshal([]byte(c.PostForm("data")), &input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid episode data: " + err.Error()})
		return
	}

	if input.EpisodeNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeNumber is required"})
		return
	}
	if input.EpisodeTitle == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeTitle is required"})
		return
	}
	if input.EpisodeType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeType is required"})
		return
	}
	validTypes := map[string]bool{"news": true, "review": true, "special": true}
	if !validTypes[input.EpisodeType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeType must be one of: news, review, special"})
		return
	}
	if input.EpisodeDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeDate is required"})
		return
	}

	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No team associated with this account"})
		return
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	pluginEnabled := false
	for _, p := range team.EnabledPlugins {
		if p == "episode_creator" {
			pluginEnabled = true
			break
		}
	}
	if !pluginEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episode_creator plugin is not enabled for this team"})
		return
	}

	if team.EpisodeCreatorURL == "" || team.EpisodeCreatorBearerToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Episode creator URL or bearer token not configured"})
		return
	}

	var uploadedImageFiles []*multipart.FileHeader
	if form, err := c.MultipartForm(); err == nil {
		uploadedImageFiles = form.File["image"]
	}
	// A draft loaded back into the form carries its image as a stored URL
	// instead of a fresh upload, so either source satisfies the requirement.
	if len(uploadedImageFiles) == 0 && len(input.ImageURLs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image is required"})
		return
	}

	imgHelper := &NewsHandler{DB: h.DB, UploadDir: h.UploadDir}
	var savedImageURLs []string
	for _, fh := range uploadedImageFiles {
		url, uploadErr := imgHelper.saveUpload(fh)
		if uploadErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed: " + uploadErr.Error()})
			return
		}
		savedImageURLs = append(savedImageURLs, url)
	}

	webhookImageURLs := mergeImageURLs(input.ImageURLs, savedImageURLs)

	webhookErr := h.sendToWebhook(ctx, &team, &input, webhookImageURLs)
	if webhookErr != nil {
		log.Printf("[EpisodeCreator] Error sending to webhook: %v", webhookErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send episode: " + webhookErr.Error()})
		return
	}

	var post *models.Post
	if input.AddSocialPosting {
		created, postErr := h.createPost(ctx, c, &input, savedImageURLs)
		if postErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Episode sent but failed to create post: " + postErr.Error()})
			return
		}
		post = created
	}

	// A draft submitted from the form keeps the content it was submitted with
	// and moves to the Posted tab instead of lingering as an open draft.
	if input.DraftID != "" {
		if draftID, idErr := primitive.ObjectIDFromHex(input.DraftID); idErr == nil {
			filter := episodeDraftScopeFilter(c)
			filter["_id"] = draftID
			fields := episodeDraftFields(&input, webhookImageURLs)
			previousImages := previousUploadURLs(ctx, h.DB.EpisodeDrafts(), filter, fields)
			if err := markDraftPosted(ctx, h.DB.EpisodeDrafts(), filter, fields); err != nil {
				log.Printf("[EpisodeCreator] Warning: could not mark draft %s as posted: %v", input.DraftID, err)
			} else {
				// The submitted images replaced the draft's own; free whatever
				// the submission dropped.
				cleanupUploads(h.DB, h.UploadDir, previousImages)
			}
		}
	}

	resp := gin.H{"message": "Episode submitted successfully"}
	if post != nil {
		resp["post"] = post
	}
	c.JSON(http.StatusOK, resp)
}

// episodeDraftFields maps a submitted episode payload onto the stored draft
// fields, so saving a draft and submitting one record the same content.
func episodeDraftFields(input *EpisodeSubmitInput, imageURLs []string) bson.M {
	return bson.M{
		"episodeNumber":     input.EpisodeNumber,
		"episodeTitle":      input.EpisodeTitle,
		"episodeType":       input.EpisodeType,
		"summary":           input.Summary,
		"episodeDate":       input.EpisodeDate,
		"gameNamePublisher": input.GameNamePublisher,
		"linkPublisher":     input.LinkPublisher,
		"linkBGG":           input.LinkBGG,
		"rules":             input.Rules,
		"scene":             input.Scene,
		"introText":         input.IntroText,
		"imageUrls":         imageURLs,
		"addSocialPosting":  input.AddSocialPosting,
		"content":           input.Content,
		"platforms":         input.Platforms,
		"scheduledAt":       input.ScheduledAt,
		"tags":              input.Tags,
		"status":            input.Status,
		"footerIds":         input.FooterIDs,
		"contentOverrides":  input.ContentOverrides,
		"accountIds":        input.AccountIDs,
		"firstComment":      input.FirstComment,
		"postType":          input.PostType,
	}
}

func (h *EpisodeHandler) sendToWebhook(ctx context.Context, team *models.Team, input *EpisodeSubmitInput, imageURLs []string) error {
	log.Printf("[EpisodeCreator] Sending episode to %s (episode %s)", team.EpisodeCreatorURL, input.EpisodeNumber)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("episodeType", input.EpisodeType)
	writer.WriteField("title", input.EpisodeTitle)
	writer.WriteField("episodeNumber", input.EpisodeNumber)
	writer.WriteField("abstractText", input.Summary)
	writer.WriteField("postingDate", input.EpisodeDate)

	if input.EpisodeType == "review" {
		writer.WriteField("gameNamePublisher", input.GameNamePublisher)
		writer.WriteField("linkPublisher", input.LinkPublisher)
		writer.WriteField("linkBGG", input.LinkBGG)
		writer.WriteField("rules", input.Rules)
		writer.WriteField("scene", input.Scene)
		writer.WriteField("introText", input.IntroText)
	}

	for _, imgURL := range imageURLs {
		if imgURL == "" {
			continue
		}
		imgPath := imgURL
		if imgPath[0] == '/' {
			imgPath = imgPath[1:]
		}
		fullPath := filepath.Join(h.UploadDir, filepath.Base(imgPath))
		fileData, err := os.ReadFile(fullPath)
		if err != nil {
			log.Printf("[EpisodeCreator] Warning: could not read image %s: %v", fullPath, err)
			continue
		}
		ext := filepath.Ext(fullPath)
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		part, _ := writer.CreateFormFile("image", filepath.Base(fullPath))
		part.Write(fileData)
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", team.EpisodeCreatorURL, &body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+team.EpisodeCreatorBearerToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("[EpisodeCreator] Success (HTTP %d)", resp.StatusCode)
	return nil
}

// --- Episode draft methods ---

func episodeDraftScopeFilter(c *gin.Context) bson.M {
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		return bson.M{"teamId": tid}
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	return bson.M{"userId": uid}
}

func (h *EpisodeHandler) SaveDraft(c *gin.Context) {
	var input EpisodeSubmitInput
	if err := json.Unmarshal([]byte(c.PostForm("data")), &input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft data: " + err.Error()})
		return
	}

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	var uploadedImageFiles []*multipart.FileHeader
	if form, err := c.MultipartForm(); err == nil {
		uploadedImageFiles = form.File["image"]
	}

	imgHelper := &NewsHandler{DB: h.DB, UploadDir: h.UploadDir}
	var savedImageURLs []string
	for _, fh := range uploadedImageFiles {
		url, uploadErr := imgHelper.saveUpload(fh)
		if uploadErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed: " + uploadErr.Error()})
			return
		}
		savedImageURLs = append(savedImageURLs, url)
	}

	allImages := mergeImageURLs(input.ImageURLs, savedImageURLs)

	platModels := make([]models.Platform, len(input.Platforms))
	for i, p := range input.Platforms {
		platModels[i] = models.Platform(p)
	}

	draft := models.EpisodeDraft{
		UserID:            objID,
		EpisodeNumber:     input.EpisodeNumber,
		EpisodeTitle:      input.EpisodeTitle,
		EpisodeType:       input.EpisodeType,
		Summary:           input.Summary,
		EpisodeDate:       input.EpisodeDate,
		GameNamePublisher: input.GameNamePublisher,
		LinkPublisher:     input.LinkPublisher,
		LinkBGG:           input.LinkBGG,
		Rules:             input.Rules,
		Scene:             input.Scene,
		IntroText:         input.IntroText,
		ImageURLs:         allImages,
		AddSocialPosting:  input.AddSocialPosting,
		Content:           input.Content,
		Platforms:         platModels,
		ScheduledAt:       input.ScheduledAt,
		Tags:              input.Tags,
		Status:            input.Status,
		FooterIDs:         input.FooterIDs,
		ContentOverrides:  input.ContentOverrides,
		AccountIDs:        input.AccountIDs,
		FirstComment:      input.FirstComment,
		PostType:          input.PostType,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		draft.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.EpisodeDrafts().InsertOne(ctx, draft)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save draft"})
		return
	}
	draft.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusOK, draft)
}

func (h *EpisodeHandler) ListDrafts(c *gin.Context) {
	filter := episodeDraftScopeFilter(c)
	posted := c.Query("posted") == "true"
	applyPostedFilter(filter, posted)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(postedSort(posted))
	cursor, err := h.DB.EpisodeDrafts().Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch drafts"})
		return
	}
	defer cursor.Close(ctx)

	var drafts []models.EpisodeDraft
	if err := cursor.All(ctx, &drafts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode drafts"})
		return
	}
	if drafts == nil {
		drafts = []models.EpisodeDraft{}
	}
	c.JSON(http.StatusOK, drafts)
}

func (h *EpisodeHandler) GetDraft(c *gin.Context) {
	draftID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
		return
	}

	filter := episodeDraftScopeFilter(c)
	filter["_id"] = draftID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var draft models.EpisodeDraft
	if err := h.DB.EpisodeDrafts().FindOne(ctx, filter).Decode(&draft); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
		return
	}
	c.JSON(http.StatusOK, draft)
}

func (h *EpisodeHandler) UpdateDraft(c *gin.Context) {
	draftID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
		return
	}

	var input EpisodeSubmitInput
	if err := json.Unmarshal([]byte(c.PostForm("data")), &input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft data: " + err.Error()})
		return
	}

	filter := episodeDraftScopeFilter(c)
	filter["_id"] = draftID

	var uploadedImageFiles []*multipart.FileHeader
	if form, err := c.MultipartForm(); err == nil {
		uploadedImageFiles = form.File["image"]
	}

	imgHelper := &NewsHandler{DB: h.DB, UploadDir: h.UploadDir}
	var savedImageURLs []string
	for _, fh := range uploadedImageFiles {
		url, uploadErr := imgHelper.saveUpload(fh)
		if uploadErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed: " + uploadErr.Error()})
			return
		}
		savedImageURLs = append(savedImageURLs, url)
	}

	allImages := mergeImageURLs(input.ImageURLs, savedImageURLs)

	update := episodeDraftFields(&input, allImages)
	update["updatedAt"] = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	previousImages := previousUploadURLs(ctx, h.DB.EpisodeDrafts(), filter, update)

	res, err := h.DB.EpisodeDrafts().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || res.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
		return
	}
	cleanupUploads(h.DB, h.UploadDir, previousImages)

	var draft models.EpisodeDraft
	h.DB.EpisodeDrafts().FindOne(ctx, bson.M{"_id": draftID}).Decode(&draft)
	c.JSON(http.StatusOK, draft)
}

func (h *EpisodeHandler) DeleteDraft(c *gin.Context) {
	draftID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
		return
	}

	filter := episodeDraftScopeFilter(c)
	filter["_id"] = draftID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	urls, err := deleteOneAndCollectUploadURLs(ctx, h.DB.EpisodeDrafts(), filter)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
		return
	}
	cleanupUploads(h.DB, h.UploadDir, urls)
	c.JSON(http.StatusOK, gin.H{"message": "Draft deleted"})
}

// DeleteDrafts removes every draft in the caller's scope on one side of the
// posted split: "?posted=true" clears the Posted tab, anything else the open
// drafts. It is the bulk counterpart of DeleteDraft.
func (h *EpisodeHandler) DeleteDrafts(c *gin.Context) {
	filter := episodeDraftScopeFilter(c)
	posted := c.Query("posted") == "true"
	applyPostedFilter(filter, posted)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	urls := collectUploadURLs(ctx, h.DB.EpisodeDrafts(), filter)

	res, err := h.DB.EpisodeDrafts().DeleteMany(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete drafts"})
		return
	}
	removedImages := cleanupUploads(h.DB, h.UploadDir, urls)
	c.JSON(http.StatusOK, gin.H{
		"message":       "Drafts deleted",
		"deletedCount":  res.DeletedCount,
		"removedImages": removedImages,
	})
}

func (h *EpisodeHandler) PostDraft(c *gin.Context) {
	draftID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
		return
	}

	filter := episodeDraftScopeFilter(c)
	filter["_id"] = draftID

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var draft models.EpisodeDraft
	if err := h.DB.EpisodeDrafts().FindOne(ctx, filter).Decode(&draft); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
		return
	}

	if draft.EpisodeNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeNumber is required"})
		return
	}
	if draft.EpisodeTitle == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeTitle is required"})
		return
	}
	if draft.EpisodeType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeType is required"})
		return
	}
	if draft.EpisodeDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeDate is required"})
		return
	}

	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No team associated with this account"})
		return
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	pluginEnabled := false
	for _, p := range team.EnabledPlugins {
		if p == "episode_creator" {
			pluginEnabled = true
			break
		}
	}
	if !pluginEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episode_creator plugin is not enabled for this team"})
		return
	}

	if team.EpisodeCreatorURL == "" || team.EpisodeCreatorBearerToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Episode creator URL or bearer token not configured"})
		return
	}

	input := &EpisodeSubmitInput{
		EpisodeNumber:     draft.EpisodeNumber,
		EpisodeTitle:      draft.EpisodeTitle,
		EpisodeType:       draft.EpisodeType,
		Summary:           draft.Summary,
		EpisodeDate:       draft.EpisodeDate,
		GameNamePublisher: draft.GameNamePublisher,
		LinkPublisher:     draft.LinkPublisher,
		LinkBGG:           draft.LinkBGG,
		Rules:             draft.Rules,
		Scene:             draft.Scene,
		IntroText:         draft.IntroText,
		AddSocialPosting:  draft.AddSocialPosting,
		Content:           draft.Content,
		Platforms:         draft.Platforms,
		ScheduledAt:       draft.ScheduledAt,
		ImageURLs:         draft.ImageURLs,
		Tags:              draft.Tags,
		Status:            draft.Status,
		FooterIDs:         draft.FooterIDs,
		ContentOverrides:  draft.ContentOverrides,
		AccountIDs:        draft.AccountIDs,
		FirstComment:      draft.FirstComment,
		PostType:          draft.PostType,
	}

	webhookErr := h.sendToWebhook(ctx, &team, input, draft.ImageURLs)
	if webhookErr != nil {
		log.Printf("[EpisodeCreator] Error sending draft to webhook: %v", webhookErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send episode: " + webhookErr.Error()})
		return
	}

	var post *models.Post
	if draft.AddSocialPosting {
		created, postErr := h.createPost(ctx, c, input, nil)
		if postErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Episode sent but failed to create post: " + postErr.Error()})
			return
		}
		post = created
	}

	if err := markDraftPosted(ctx, h.DB.EpisodeDrafts(), bson.M{"_id": draftID}, nil); err != nil {
		log.Printf("[EpisodeCreator] Warning: could not mark draft %s as posted: %v", draftID.Hex(), err)
	}

	resp := gin.H{"message": "Episode submitted successfully"}
	if post != nil {
		resp["post"] = post
	}
	c.JSON(http.StatusOK, resp)
}

func (h *EpisodeHandler) createPost(ctx context.Context, c *gin.Context, input *EpisodeSubmitInput, imageURLs []string) (*models.Post, error) {
	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	scheduledAt, err := time.Parse(time.RFC3339, input.ScheduledAt)
	if err != nil {
		return nil, fmt.Errorf("invalid scheduledAt format, use RFC3339")
	}

	status := models.PostStatusScheduled
	if input.Status != "" {
		status = input.Status
	}

	postType := input.PostType
	if postType == "" {
		postType = models.PostTypePost
	}

	allImages := mergeImageURLs(input.ImageURLs, imageURLs)

	post := models.Post{
		UserID:           objID,
		PostType:         postType,
		Content:          input.Content,
		FirstComment:     input.FirstComment,
		Platforms:        input.Platforms,
		ScheduledAt:      scheduledAt,
		Status:           status,
		Tags:             input.Tags,
		AccountIDs:       input.AccountIDs,
		ImageURLs:        allImages,
		FooterIDs:        input.FooterIDs,
		ContentOverrides: input.ContentOverrides,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		post.TeamID = &tid
	}

	result, err := h.DB.Posts().InsertOne(ctx, post)
	if err != nil {
		return nil, fmt.Errorf("failed to create post: %w", err)
	}
	post.ID = result.InsertedID.(primitive.ObjectID)

	return &post, nil
}
