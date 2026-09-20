package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AgentHandler struct {
	DB        *database.MongoDB
	UploadDir string
}

type AgentGenerateInput struct {
	URL         string `json:"url"`
	Description string `json:"description"`
	EntityType  string `json:"entityType"`
}

var defaultAgentSystemPrompt string

func init() {
	defaultAgentSystemPrompt = loadAgentInstructions()
}

func loadAgentInstructions() string {
	data, err := os.ReadFile("agent-instructions.md")
	if err != nil {
		// Try the dist directory (embedded frontend build)
		data, err = os.ReadFile("dist/agent-instructions.md")
	}
	if err == nil && len(data) > 0 {
		return string(data) + "\n\nReply with ONLY valid JSON, no markdown code fences, no commentary."
	}
	return fallbackAgentSystemPrompt
}

const fallbackAgentSystemPrompt = `You are a content creation assistant for a social media management platform. Given a URL and/or a description, create engaging content suitable for social media posting. All created content will be saved as drafts for human review before publishing.

When given a URL, analyze its content (provided to you as extracted page metadata) and create content based on it.

Based on the requested entity type, return a JSON object with ONLY the following fields:

For "news":
{
  "newsTagline": "A compelling headline/tagline for the news item",
  "articleUrl": "The source URL",
  "shownotes": "Key points and summary of the article",
  "content": "Ready-to-post social media text (max 280 characters)"
}

For "episode":
{
  "episodeTitle": "An engaging episode title",
  "episodeType": "news",
  "summary": "A comprehensive summary",
  "introText": "An introduction for the episode",
  "content": "Ready-to-post social media text (max 280 characters)",
  "gameNamePublisher": "Game Name (Publisher) - only for review type",
  "linkPublisher": "Publisher URL - only for review type",
  "linkBGG": "BoardGameGeek URL - only for review type",
  "rules": "Game rules overview in German - only for review type",
  "scene": "Humorous radio play scene with Jutta and Michael - only for review type",
  "episodeDate": "YYYY-MM-DDTHH:MM format"
}

For "post":
{
  "content": "Ready-to-post social media text optimized for engagement (max 280 characters)"
}

Always write in a professional but engaging tone. Include relevant context from the source material.
Reply with ONLY valid JSON, no markdown code fences, no commentary.`

func (h *AgentHandler) GetInstructions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instructions": defaultAgentSystemPrompt})
}

func (h *AgentHandler) Generate(c *gin.Context) {
	var input AgentGenerateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.URL == "" && input.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Either url or description is required"})
		return
	}
	if input.EntityType != "news" && input.EntityType != "episode" && input.EntityType != "post" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "entityType must be news, episode, or post"})
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

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	pluginEnabled := false
	for _, p := range team.EnabledPlugins {
		if p == "agent" {
			pluginEnabled = true
			break
		}
	}
	if !pluginEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Agent plugin is not enabled for this team"})
		return
	}

	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil || settings.OpenRouterAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OpenRouter is not configured. Ask your admin to set up an OpenRouter API key in Settings."})
		return
	}

	var pageInfo string
	var ogImage string
	if input.URL != "" {
		title, description, img, body := fetchPageMetadata(ctx, input.URL)
		ogImage = img
		var parts []string
		parts = append(parts, fmt.Sprintf("URL: %s", input.URL))
		if title != "" {
			parts = append(parts, fmt.Sprintf("Page Title: %s", title))
		}
		if description != "" {
			parts = append(parts, fmt.Sprintf("Description: %s", description))
		}
		if body != "" {
			parts = append(parts, fmt.Sprintf("Page Content:\n%s", body))
		}
		pageInfo = strings.Join(parts, "\n")
	}

	var userPrompt string
	if pageInfo != "" && input.Description != "" {
		userPrompt = fmt.Sprintf("Source URL metadata:\n%s\n\nAdditional instructions: %s\n\nCreate a %s entity from this.", pageInfo, input.Description, input.EntityType)
	} else if pageInfo != "" {
		userPrompt = fmt.Sprintf("Source URL metadata:\n%s\n\nCreate a %s entity from this.", pageInfo, input.EntityType)
	} else {
		userPrompt = fmt.Sprintf("Description: %s\n\nCreate a %s entity from this.", input.Description, input.EntityType)
	}

	systemPrompt := team.AgentSystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultAgentSystemPrompt
	}
	if settings.AILanguage != "" {
		systemPrompt += fmt.Sprintf("\n\nWrite in %s.", settings.AILanguage)
	}

	model := settings.OpenRouterModel
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+settings.OpenRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to reach OpenRouter: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("OpenRouter returned %d: %s", resp.StatusCode, string(body))})
		return
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Invalid response from OpenRouter"})
		return
	}

	aiContent := result.Choices[0].Message.Content
	aiContent = strings.TrimSpace(aiContent)
	aiContent = strings.TrimPrefix(aiContent, "```json")
	aiContent = strings.TrimPrefix(aiContent, "```")
	aiContent = strings.TrimSuffix(aiContent, "```")
	aiContent = strings.TrimSpace(aiContent)

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	var imageURLs []string
	if ogImage != "" {
		saved, saveErr := downloadAndSaveImage(ctx, h.DB, h.UploadDir, ogImage)
		if saveErr != nil {
			log.Printf("[Agent] Warning: failed to download og:image %s: %v", ogImage, saveErr)
		} else {
			imageURLs = append(imageURLs, saved)
		}
	}

	switch input.EntityType {
	case "news":
		draft, err := h.createNewsDraft(ctx, aiContent, objID, &teamID, input.URL, imageURLs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "AI generated content but failed to save draft: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"entityType":   "news",
			"draft":        draft,
			"imageSources": describeImageSources(ogImage),
		})

	case "episode":
		draft, err := h.createEpisodeDraft(ctx, aiContent, objID, &teamID, imageURLs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "AI generated content but failed to save draft: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"entityType":   "episode",
			"draft":        draft,
			"imageSources": describeImageSources(ogImage),
		})

	case "post":
		draft, err := h.createPostDraft(ctx, aiContent, objID, &teamID, imageURLs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "AI generated content but failed to save draft: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"entityType":   "post",
			"draft":        draft,
			"imageSources": describeImageSources(ogImage),
		})
	}
}

func (h *AgentHandler) createNewsDraft(ctx context.Context, aiContent string, userID primitive.ObjectID, teamID *primitive.ObjectID, sourceURL string, imageURLs []string) (*models.NewsDraft, error) {
	var parsed struct {
		NewsTagline string `json:"newsTagline"`
		ArticleURL  string `json:"articleUrl"`
		Shownotes   string `json:"shownotes"`
		Content     string `json:"content"`
	}
	if err := json.Unmarshal([]byte(aiContent), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	if parsed.ArticleURL == "" {
		parsed.ArticleURL = sourceURL
	}

	draft := models.NewsDraft{
		UserID:      userID,
		TeamID:      teamID,
		NewsTagline: parsed.NewsTagline,
		ArticleURL:  parsed.ArticleURL,
		Shownotes:   parsed.Shownotes,
		Content:     parsed.Content,
		ImageURLs:   imageURLs,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result, err := h.DB.NewsDrafts().InsertOne(ctx, draft)
	if err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}
	draft.ID = result.InsertedID.(primitive.ObjectID)
	return &draft, nil
}

func (h *AgentHandler) createEpisodeDraft(ctx context.Context, aiContent string, userID primitive.ObjectID, teamID *primitive.ObjectID, imageURLs []string) (*models.EpisodeDraft, error) {
	var parsed struct {
		EpisodeTitle      string `json:"episodeTitle"`
		EpisodeType       string `json:"episodeType"`
		Summary           string `json:"summary"`
		EpisodeDate       string `json:"episodeDate"`
		IntroText         string `json:"introText"`
		GameNamePublisher string `json:"gameNamePublisher"`
		LinkPublisher     string `json:"linkPublisher"`
		LinkBGG           string `json:"linkBGG"`
		Rules             string `json:"rules"`
		Scene             string `json:"scene"`
		Content           string `json:"content"`
	}
	if err := json.Unmarshal([]byte(aiContent), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	if parsed.EpisodeType == "" {
		parsed.EpisodeType = "news"
	}

	draft := models.EpisodeDraft{
		UserID:            userID,
		TeamID:            teamID,
		EpisodeTitle:      parsed.EpisodeTitle,
		EpisodeType:       parsed.EpisodeType,
		Summary:           parsed.Summary,
		EpisodeDate:       parsed.EpisodeDate,
		IntroText:         parsed.IntroText,
		GameNamePublisher: parsed.GameNamePublisher,
		LinkPublisher:     parsed.LinkPublisher,
		LinkBGG:           parsed.LinkBGG,
		Rules:             parsed.Rules,
		Scene:             parsed.Scene,
		Content:           parsed.Content,
		ImageURLs:         imageURLs,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	result, err := h.DB.EpisodeDrafts().InsertOne(ctx, draft)
	if err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}
	draft.ID = result.InsertedID.(primitive.ObjectID)
	return &draft, nil
}

func (h *AgentHandler) createPostDraft(ctx context.Context, aiContent string, userID primitive.ObjectID, teamID *primitive.ObjectID, imageURLs []string) (*models.Post, error) {
	var parsed struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(aiContent), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	post := models.Post{
		UserID:    userID,
		PostType:  models.PostTypePost,
		Content:   parsed.Content,
		Status:    models.PostStatusDraft,
		ImageURLs: imageURLs,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if teamID != nil {
		post.TeamID = teamID
	}

	result, err := h.DB.Posts().InsertOne(ctx, post)
	if err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}
	post.ID = result.InsertedID.(primitive.ObjectID)
	return &post, nil
}

func fetchPageMetadata(ctx context.Context, pageURL string) (title, description, ogImage, bodyText string) {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SocialPod/1.0)")
	req.Header.Set("Accept", "text/html")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	htmlBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return
	}
	html := string(htmlBytes)

	titleRe := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	if m := titleRe.FindStringSubmatch(html); len(m) > 1 {
		title = strings.TrimSpace(m[1])
	}

	ogTitleRe := regexp.MustCompile(`(?i)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']`)
	if m := ogTitleRe.FindStringSubmatch(html); len(m) > 1 {
		title = strings.TrimSpace(m[1])
	}

	descRe := regexp.MustCompile(`(?i)<meta[^>]+(?:name=["']description["']|property=["']og:description["'])[^>]+content=["']([^"']+)["']`)
	if m := descRe.FindStringSubmatch(html); len(m) > 1 {
		description = strings.TrimSpace(m[1])
	}

	ogImageRe := regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	if m := ogImageRe.FindStringSubmatch(html); len(m) > 1 {
		ogImage = strings.TrimSpace(m[1])
	}

	tagRe := regexp.MustCompile(`<[^>]+>`)
	text := tagRe.ReplaceAllString(html, " ")
	spaceRe := regexp.MustCompile(`\s+`)
	text = spaceRe.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	if len(text) > 3000 {
		text = text[:3000]
	}
	bodyText = text

	return
}

func downloadAndSaveImage(ctx context.Context, db *database.MongoDB, uploadDir string, imageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SocialPod/1.0)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching image", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", err
	}

	ct := resp.Header.Get("Content-Type")
	ext := ".jpg"
	switch {
	case strings.Contains(ct, "png"):
		ext = ".png"
	case strings.Contains(ct, "gif"):
		ext = ".gif"
	case strings.Contains(ct, "webp"):
		ext = ".webp"
	}

	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(uploadDir, filename)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}

	db.Uploads().InsertOne(ctx, models.Upload{
		Filename:    filename,
		ContentType: ct,
		Data:        data,
		Size:        int64(len(data)),
		CreatedAt:   time.Now(),
	})

	return "/api/uploads/" + filename, nil
}

func describeImageSources(ogImage string) []map[string]string {
	if ogImage == "" {
		return nil
	}
	return []map[string]string{
		{
			"type":   "og:image",
			"url":    ogImage,
			"status": "downloaded",
		},
	}
}
