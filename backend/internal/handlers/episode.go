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
	SuffixIDs        map[string]string `json:"suffixIds,omitempty"`
	ContentOverrides map[string]string `json:"contentOverrides,omitempty"`
	AccountIDs       map[string]string `json:"accountIds,omitempty"`
	FirstComment     string            `json:"firstComment,omitempty"`
	PostType         models.PostType   `json:"postType,omitempty"`
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
	if len(uploadedImageFiles) == 0 {
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

	webhookErr := h.sendToWebhook(ctx, &team, &input, savedImageURLs)
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

	resp := gin.H{"message": "Episode submitted successfully"}
	if post != nil {
		resp["post"] = post
	}
	c.JSON(http.StatusOK, resp)
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

	allImages := append(input.ImageURLs, imageURLs...)

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
		SuffixIDs:        input.SuffixIDs,
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
