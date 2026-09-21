package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

var bggURLRe = regexp.MustCompile(`boardgamegeek\.com/boardgame[^/]*/(\d+)`)

// CaptureHandler handles the Chrome extension capture endpoint.
type CaptureHandler struct {
	DB        *database.MongoDB
	UploadDir string
	BGG       *BGGHandler
}

const defaultSystemPrompt = `You are a content creation assistant for a social media management platform. Given a URL and/or a description, create engaging content suitable for social media posting. All created content will be saved as drafts for human review before publishing.

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

type CaptureImage struct {
	Data     string `json:"data"`
	Filename string `json:"filename"`
}

type CaptureInput struct {
	URL         string         `json:"url"`
	EntityType  string         `json:"entityType"`
	Description string         `json:"description"`
	PageTitle   string         `json:"pageTitle"`
	Images      []CaptureImage `json:"images"`
}

func (h *CaptureHandler) Capture(c *gin.Context) {
	var input CaptureInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	if input.EntityType == "" {
		input.EntityType = "news"
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

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil || settings.OpenRouterAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OpenRouter is not configured"})
		return
	}

	// Step 1: Save images sent by the Chrome extension as base64 data.
	// The extension downloads images in the browser (avoiding proxy/bot issues)
	// and sends the raw data here.
	episodeType := "news_entry"
	if input.EntityType == "episode" {
		episodeType = "news"
	}

	var imageURLs []string
	if len(input.Images) > 0 {
		imageURLs = h.saveClientImages(ctx, c, input.Images, episodeType)
		log.Printf("[Capture] Image URLs for draft: %v", imageURLs)
	}

	// Step 2: Fetch page info for AI content generation
	var pageInfo string
	if input.URL != "" {
		if m := bggURLRe.FindStringSubmatch(input.URL); m != nil {
			pageInfo, _ = fetchBGGPageInfo(ctx, m[1], input.URL, settings.BGGAPIToken)
		}
		if pageInfo == "" {
			var parts []string
			parts = append(parts, fmt.Sprintf("URL: %s", input.URL))
			if input.PageTitle != "" {
				parts = append(parts, fmt.Sprintf("Page Title: %s", input.PageTitle))
			}
			if input.Description != "" {
				parts = append(parts, fmt.Sprintf("Additional context: %s", input.Description))
			}
			html, _ := fetchPageHTML(ctx, input.URL)
			if html != "" {
				_, description, _, body := extractPageMetadata(html)
				if description != "" {
					parts = append(parts, fmt.Sprintf("Description: %s", description))
				}
				if body != "" {
					parts = append(parts, fmt.Sprintf("Page Content:\n%s", body))
				}
			}
			pageInfo = strings.Join(parts, "\n")
		}
	}

	// Step 3: Generate content with AI
	var userPrompt string
	if pageInfo != "" && input.Description != "" {
		userPrompt = fmt.Sprintf("Source URL metadata:\n%s\n\nAdditional instructions: %s\n\nCreate a %s entity from this.", pageInfo, input.Description, input.EntityType)
	} else if pageInfo != "" {
		userPrompt = fmt.Sprintf("Source URL metadata:\n%s\n\nCreate a %s entity from this.", pageInfo, input.EntityType)
	} else {
		userPrompt = fmt.Sprintf("URL: %s\nPage Title: %s\n\nCreate a %s entity from this.", input.URL, input.PageTitle, input.EntityType)
	}

	systemPrompt := team.AgentSystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
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

	switch input.EntityType {
	case "news":
		draft, err := h.createNewsDraft(ctx, aiContent, objID, &teamID, input.URL, imageURLs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save draft: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"entityType": "news", "draft": draft})
	case "episode":
		draft, err := h.createEpisodeDraft(ctx, aiContent, objID, &teamID, imageURLs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save draft: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"entityType": "episode", "draft": draft})
	case "post":
		draft, err := h.createPostDraft(ctx, aiContent, objID, &teamID, imageURLs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save draft: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"entityType": "post", "draft": draft})
	}
}

// saveClientImages decodes base64 image data sent by the Chrome extension,
// applies the overlay if configured, and saves the images.
func (h *CaptureHandler) saveClientImages(ctx context.Context, c *gin.Context, images []CaptureImage, episodeType string) []string {
	for _, img := range images {
		if img.Data == "" {
			continue
		}

		raw, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			log.Printf("[Capture] Failed to decode base64 image: %v", err)
			continue
		}

		ct := http.DetectContentType(raw)
		if !strings.HasPrefix(ct, "image/") {
			log.Printf("[Capture] Skipping non-image content type: %s", ct)
			continue
		}

		if h.BGG != nil {
			processed, procErr := h.BGG.processImageBytes(ctx, c, raw, episodeType)
			if procErr != nil {
				log.Printf("[Capture] Overlay processing failed: %v (saving original)", procErr)
			} else {
				raw = processed
				ct = "image/jpeg"
			}
		}

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
		if err := os.MkdirAll(h.UploadDir, 0o755); err != nil {
			log.Printf("[Capture] Failed to create upload dir: %v", err)
			continue
		}
		dst := filepath.Join(h.UploadDir, filename)
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			log.Printf("[Capture] Failed to write image file: %v", err)
			continue
		}
		h.DB.Uploads().InsertOne(ctx, models.Upload{
			Filename:    filename,
			ContentType: ct,
			Data:        raw,
			Size:        int64(len(raw)),
			CreatedAt:   time.Now(),
		})
		return []string{"/api/uploads/" + filename}
	}

	log.Printf("[Capture] No valid images from %d client-provided entries", len(images))
	return nil
}

func (h *CaptureHandler) createNewsDraft(ctx context.Context, aiContent string, userID primitive.ObjectID, teamID *primitive.ObjectID, sourceURL string, imageURLs []string) (*models.NewsDraft, error) {
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

func (h *CaptureHandler) createEpisodeDraft(ctx context.Context, aiContent string, userID primitive.ObjectID, teamID *primitive.ObjectID, imageURLs []string) (*models.EpisodeDraft, error) {
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

func (h *CaptureHandler) createPostDraft(ctx context.Context, aiContent string, userID primitive.ObjectID, teamID *primitive.ObjectID, imageURLs []string) (*models.Post, error) {
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

// --- Shared utility functions used by capture, MCP, and other handlers ---

// extractMetaContent finds a <meta> tag whose property or name attribute
// matches one of the given names and returns its content attribute value.
func extractMetaContent(html string, names ...string) string {
	metaRe := regexp.MustCompile(`(?i)<meta\s[^>]*>`)
	for _, tag := range metaRe.FindAllString(html, -1) {
		contentRe := regexp.MustCompile(`(?i)content=["']([^"']+)["']`)
		cm := contentRe.FindStringSubmatch(tag)
		if len(cm) < 2 {
			continue
		}
		for _, name := range names {
			propRe := regexp.MustCompile(`(?i)(?:property|name)=["']` + regexp.QuoteMeta(name) + `["']`)
			if propRe.MatchString(tag) {
				return strings.TrimSpace(cm[1])
			}
		}
	}
	return ""
}

func resolveURL(rawURL, baseURL string) string {
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return rawURL
	}
	ref, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return base.ResolveReference(ref).String()
}

type imageCandidate struct {
	url   string
	score int
}

var (
	imgTagRe    = regexp.MustCompile(`(?i)<img\s[^>]*>`)
	imgSrcRe    = regexp.MustCompile(`(?i)(?:src|data-src|data-lazy-src|data-original)=["']([^"']+)["']`)
	imgWidthRe  = regexp.MustCompile(`(?i)width=["']?(\d+)`)
	imgHeightRe = regexp.MustCompile(`(?i)height=["']?(\d+)`)
	imgAltRe    = regexp.MustCompile(`(?i)alt=["']([^"']*?)["']`)
	imgClassRe  = regexp.MustCompile(`(?i)class=["']([^"']*?)["']`)
	imgSrcsetRe = regexp.MustCompile(`(?i)srcset=["']([^"']+)["']`)

	negativeImagePatterns = []string{
		"logo", "icon", "avatar", "sprite", "pixel", "tracking",
		"button", "arrow", "spinner", "loading", "placeholder",
		"badge", "flag", "emoji", "social-", "share", "gravatar", "favicon",
	}
	positiveImagePatterns = []string{
		"hero", "featured", "article", "cover", "banner",
		"product", "content", "thumbnail", "post", "entry", "news",
		"header-image", "wp-content", "uploads", "produktbanner",
	}
)

func extractJSONLDImage(html, pageURL string) string {
	jsonLDRe := regexp.MustCompile(`(?i)<script[^>]+type=["']application/ld\+json["'][^>]*>([\s\S]*?)</script>`)
	for _, m := range jsonLDRe.FindAllStringSubmatch(html, 10) {
		if len(m) < 2 {
			continue
		}
		if img := parseJSONLDImage(m[1]); img != "" {
			return resolveURL(img, pageURL)
		}
	}
	return ""
}

func parseJSONLDImage(jsonStr string) string {
	var data map[string]any
	if json.Unmarshal([]byte(jsonStr), &data) == nil {
		if img := jsonLDImageField(data); img != "" {
			return img
		}
		if graph, ok := data["@graph"].([]any); ok {
			for _, item := range graph {
				if obj, ok := item.(map[string]any); ok {
					if img := jsonLDImageField(obj); img != "" {
						return img
					}
				}
			}
		}
	}
	var arr []map[string]any
	if json.Unmarshal([]byte(jsonStr), &arr) == nil {
		for _, data := range arr {
			if img := jsonLDImageField(data); img != "" {
				return img
			}
		}
	}
	return ""
}

func jsonLDImageField(data map[string]any) string {
	img, ok := data["image"]
	if !ok {
		return ""
	}
	switch v := img.(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
			if obj, ok := v[0].(map[string]any); ok {
				if u, ok := obj["url"].(string); ok {
					return u
				}
			}
		}
	case map[string]any:
		if u, ok := v["url"].(string); ok {
			return u
		}
	}
	return ""
}

func scoreImgTags(html, pageURL string) []imageCandidate {
	tags := imgTagRe.FindAllString(html, 100)
	if len(tags) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var candidates []imageCandidate

	for i, tag := range tags {
		sm := imgSrcRe.FindStringSubmatch(tag)
		if len(sm) < 2 {
			continue
		}
		rawSrc := strings.TrimSpace(sm[1])
		rawSrc = unwrapNextImageURL(rawSrc)
		imgURL := resolveURL(rawSrc, pageURL)
		if imgURL == "" || strings.HasPrefix(imgURL, "data:") {
			continue
		}
		imgURL = unwrapNextImageURL(imgURL)
		if seen[imgURL] {
			continue
		}
		seen[imgURL] = true

		score := 10

		if i < 5 {
			score += 2
		}

		w, h := 0, 0
		if wm := imgWidthRe.FindStringSubmatch(tag); len(wm) > 1 {
			w, _ = strconv.Atoi(wm[1])
		}
		if hm := imgHeightRe.FindStringSubmatch(tag); len(hm) > 1 {
			h, _ = strconv.Atoi(hm[1])
		}

		if (w > 0 && w < 50) || (h > 0 && h < 50) {
			score -= 15
		}
		if w == 1 || h == 1 {
			score -= 30
		}
		if w >= 200 || h >= 200 {
			score += 3
		}
		if w >= 400 || h >= 400 {
			score += 5
		}
		if w >= 600 {
			score += 3
		}

		if am := imgAltRe.FindStringSubmatch(tag); len(am) > 1 {
			alt := strings.TrimSpace(am[1])
			if len(alt) > 10 {
				score += 3
			} else if len(alt) > 0 {
				score += 1
			}
		}

		lowerTag := strings.ToLower(tag)
		lowerURL := strings.ToLower(imgURL)

		for _, p := range negativeImagePatterns {
			if strings.Contains(lowerURL, p) || strings.Contains(lowerTag, p) {
				score -= 10
				break
			}
		}
		for _, p := range positiveImagePatterns {
			if strings.Contains(lowerURL, p) || strings.Contains(lowerTag, p) {
				score += 3
				break
			}
		}

		if strings.HasSuffix(lowerURL, ".svg") {
			score -= 5
		}

		if imgSrcsetRe.MatchString(tag) {
			score += 3
		}

		if cm := imgClassRe.FindStringSubmatch(tag); len(cm) > 1 {
			cls := strings.ToLower(cm[1])
			for _, p := range positiveImagePatterns {
				if strings.Contains(cls, p) {
					score += 3
					break
				}
			}
		}

		candidates = append(candidates, imageCandidate{url: imgURL, score: score})
	}

	return candidates
}

// extractContentImageCandidates returns image URLs from the HTML, ordered by
// quality. It tries og:image, twitter:image, JSON-LD, Next.js __NEXT_DATA__,
// srcset attributes, and scored <img> tags.
func extractContentImageCandidates(html, pageURL string) []string {
	var candidates []string
	seen := make(map[string]bool)

	add := func(u string) {
		if u != "" && !seen[u] && !strings.HasPrefix(u, "data:") {
			seen[u] = true
			candidates = append(candidates, u)
		}
	}

	if img := extractMetaContent(html, "og:image"); img != "" {
		add(resolveURL(img, pageURL))
	}
	if img := extractMetaContent(html, "twitter:image", "twitter:image:src"); img != "" {
		add(resolveURL(img, pageURL))
	}

	linkImageRe := regexp.MustCompile(`(?i)<link[^>]+rel=["']image_src["'][^>]+href=["']([^"']+)["']`)
	if m := linkImageRe.FindStringSubmatch(html); len(m) > 1 {
		add(resolveURL(m[1], pageURL))
	}

	if img := extractJSONLDImage(html, pageURL); img != "" {
		add(img)
	}

	for _, img := range extractNextDataImages(html, pageURL) {
		add(img)
	}

	for _, img := range extractSrcsetURLs(html, pageURL) {
		add(img)
	}

	scored := scoreImgTags(html, pageURL)
	for i := range scored {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	for _, c := range scored {
		if c.score >= 5 {
			add(c.url)
		}
	}

	return candidates
}

var nextDataRe = regexp.MustCompile(`(?i)<script[^>]+id=["']__NEXT_DATA__["'][^>]*>([\s\S]*?)</script>`)

func extractNextDataImages(html, pageURL string) []string {
	m := nextDataRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil
	}

	var data map[string]any
	if json.Unmarshal([]byte(m[1]), &data) != nil {
		return nil
	}

	var urls []string
	seen := make(map[string]bool)
	collectImageURLs(data, pageURL, seen, &urls, 0)
	return urls
}

func collectImageURLs(v any, pageURL string, seen map[string]bool, out *[]string, depth int) {
	if depth > 15 || len(*out) >= 20 {
		return
	}
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			lk := strings.ToLower(k)
			if isImageURLKey(lk) {
				if s, ok := child.(string); ok && isPlausibleImageURL(s) {
					resolved := resolveURL(s, pageURL)
					if !seen[resolved] {
						seen[resolved] = true
						*out = append(*out, resolved)
					}
				}
			}
			collectImageURLs(child, pageURL, seen, out, depth+1)
		}
	case []any:
		for _, item := range val {
			collectImageURLs(item, pageURL, seen, out, depth+1)
		}
	}
}

var imageURLKeySet = map[string]bool{
	"url": true, "src": true, "image": true, "imageurl": true,
	"file": true, "thumbnail": true, "banner": true, "cover": true,
	"poster": true, "hero": true, "og_image": true, "ogimage": true,
	"featured_image": true, "featuredimage": true, "photo": true,
	"picture": true, "media_url": true, "mediaurl": true,
}

func isImageURLKey(key string) bool {
	return imageURLKeySet[key]
}

var imageExtRe = regexp.MustCompile(`(?i)\.(jpe?g|png|gif|webp|bmp|avif|svg)(\?|$)`)

func isPlausibleImageURL(s string) bool {
	if s == "" || strings.HasPrefix(s, "data:") {
		return false
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "//") || strings.HasPrefix(s, "/") {
		if imageExtRe.MatchString(s) {
			return true
		}
		lowerS := strings.ToLower(s)
		if strings.Contains(lowerS, "/image") || strings.Contains(lowerS, "/upload") || strings.Contains(lowerS, "/media") || strings.Contains(lowerS, "/photo") || strings.Contains(lowerS, "/asset") {
			return true
		}
		return true
	}
	return false
}

func unwrapNextImageURL(imgURL string) string {
	if !strings.Contains(imgURL, "/_next/image") {
		return imgURL
	}
	parsed, err := url.Parse(imgURL)
	if err != nil {
		return imgURL
	}
	if real := parsed.Query().Get("url"); real != "" {
		decoded, err := url.QueryUnescape(real)
		if err == nil && decoded != "" {
			return decoded
		}
		return real
	}
	return imgURL
}

var srcsetEntryRe = regexp.MustCompile(`(\S+)\s+(\d+)w`)

func extractSrcsetURLs(html, pageURL string) []string {
	tags := imgTagRe.FindAllString(html, 100)
	seen := make(map[string]bool)
	var results []string

	for _, tag := range tags {
		sm := imgSrcsetRe.FindStringSubmatch(tag)
		if len(sm) < 2 {
			continue
		}
		entries := srcsetEntryRe.FindAllStringSubmatch(sm[1], -1)
		if len(entries) == 0 {
			continue
		}
		bestURL := ""
		bestW := 0
		for _, e := range entries {
			w, _ := strconv.Atoi(e[2])
			rawURL := unwrapNextImageURL(strings.TrimSpace(e[1]))
			if strings.HasPrefix(rawURL, "data:") {
				continue
			}
			if w > bestW {
				bestW = w
				bestURL = rawURL
			}
		}
		if bestURL != "" {
			resolved := resolveURL(bestURL, pageURL)
			if !seen[resolved] {
				seen[resolved] = true
				results = append(results, resolved)
			}
		}
	}
	return results
}

func fetchPageHTML(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,de;q=0.8")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	htmlBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return "", err
	}

	return string(htmlBytes), nil
}

func extractPageMetadata(html string) (title, description, ogImage, bodyText string) {
	titleRe := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	if m := titleRe.FindStringSubmatch(html); len(m) > 1 {
		title = strings.TrimSpace(m[1])
	}

	if v := extractMetaContent(html, "og:title"); v != "" {
		title = v
	}

	if v := extractMetaContent(html, "description", "og:description"); v != "" {
		description = v
	}

	ogImage = extractMetaContent(html, "og:image")

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
	req.Header.Set("User-Agent", browserUserAgent)

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

func fetchBGGPageInfo(ctx context.Context, gameID, pageURL, bggToken string) (pageInfo string, imageURL string) {
	item, err := fetchBGGItem(ctx, gameID, bggToken)
	if err != nil {
		log.Printf("[Capture] BGG API fetch failed for game %s: %v", gameID, err)
		return "", ""
	}

	title := ""
	for _, n := range item.Names {
		if n.Type == "primary" {
			title = n.Value
			break
		}
	}
	if title == "" && len(item.Names) > 0 {
		title = item.Names[0].Value
	}

	var designers, artists, publishers, categories, mechanics []string
	for _, link := range item.Links {
		switch link.Type {
		case "boardgamedesigner":
			designers = append(designers, link.Value)
		case "boardgameartist":
			artists = append(artists, link.Value)
		case "boardgamepublisher":
			publishers = append(publishers, link.Value)
		case "boardgamecategory":
			categories = append(categories, link.Value)
		case "boardgamemechanic":
			mechanics = append(mechanics, link.Value)
		}
	}

	description := cleanBGGText(item.Desc)
	if len(description) > 2000 {
		description = description[:2000]
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("URL: %s", pageURL))
	parts = append(parts, fmt.Sprintf("Source: BoardGameGeek (BGG)"))
	parts = append(parts, fmt.Sprintf("Game Title: %s", title))
	if item.YearPub.Value != "" {
		parts = append(parts, fmt.Sprintf("Year Published: %s", item.YearPub.Value))
	}
	if len(designers) > 0 {
		parts = append(parts, fmt.Sprintf("Designers: %s", strings.Join(designers, ", ")))
	}
	if len(artists) > 0 {
		parts = append(parts, fmt.Sprintf("Artists: %s", strings.Join(artists, ", ")))
	}
	if len(publishers) > 0 {
		parts = append(parts, fmt.Sprintf("Publishers: %s", strings.Join(publishers, ", ")))
	}
	if len(categories) > 0 {
		parts = append(parts, fmt.Sprintf("Categories: %s", strings.Join(categories, ", ")))
	}
	if len(mechanics) > 0 {
		parts = append(parts, fmt.Sprintf("Mechanics: %s", strings.Join(mechanics, ", ")))
	}
	if item.MinPlayers.Value != "" && item.MaxPlayers.Value != "" {
		parts = append(parts, fmt.Sprintf("Players: %s–%s", item.MinPlayers.Value, item.MaxPlayers.Value))
	}
	if item.MinTime.Value != "" && item.MaxTime.Value != "" {
		parts = append(parts, fmt.Sprintf("Playtime: %s–%s minutes", item.MinTime.Value, item.MaxTime.Value))
	}
	if item.MinAge.Value != "" {
		parts = append(parts, fmt.Sprintf("Minimum Age: %s+", item.MinAge.Value))
	}
	if item.Stats.Ratings.Average.Value != "" {
		parts = append(parts, fmt.Sprintf("BGG Rating: %s", trimFloat(item.Stats.Ratings.Average.Value)))
	}
	if item.Stats.Ratings.Weight.Value != "" {
		parts = append(parts, fmt.Sprintf("Complexity Weight: %s", trimFloat(item.Stats.Ratings.Weight.Value)))
	}
	if description != "" {
		parts = append(parts, fmt.Sprintf("Description:\n%s", description))
	}

	imgURL := strings.TrimSpace(item.Image)
	if imgURL == "" {
		imgURL = strings.TrimSpace(item.Thumbnail)
	}
	if imgURL != "" && strings.HasPrefix(imgURL, "//") {
		imgURL = "https:" + imgURL
	}

	return strings.Join(parts, "\n"), imgURL
}
