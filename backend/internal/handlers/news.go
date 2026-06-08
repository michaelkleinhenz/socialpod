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

type NewsHandler struct {
	DB        *database.MongoDB
	UploadDir string
}

type NewsSubmitInput struct {
	EpisodeNumber    string            `json:"episodeNumber"`
	NewsTagline      string            `json:"newsTagline"`
	ArticleURL       string            `json:"articleUrl"`
	Shownotes        string            `json:"shownotes,omitempty"`
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

func (h *NewsHandler) Submit(c *gin.Context) {
	var input NewsSubmitInput
	if err := json.Unmarshal([]byte(c.PostForm("data")), &input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid news data: " + err.Error()})
		return
	}

	// Validate mandatory fields.
	if input.EpisodeNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "episodeNumber is required"})
		return
	}
	if input.NewsTagline == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "newsTagline is required"})
		return
	}
	if input.ArticleURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "articleUrl is required"})
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

	// Verify plugin is enabled.
	pluginEnabled := false
	for _, p := range team.EnabledPlugins {
		if p == "news_creator" {
			pluginEnabled = true
			break
		}
	}
	if !pluginEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "news_creator plugin is not enabled for this team"})
		return
	}

	if team.NewsCreatorURL == "" || team.NewsCreatorBearerToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "News creator URL or bearer token not configured"})
		return
	}

	// Collect uploaded image files for both the n8n request and optional post creation.
	var uploadedImageFiles []*multipart.FileHeader
	if form, err := c.MultipartForm(); err == nil {
		uploadedImageFiles = form.File["image"]
	}

	// Save images to disk (needed for both n8n forwarding and post creation).
	var savedImageURLs []string
	for _, fh := range uploadedImageFiles {
		url, uploadErr := h.saveUpload(fh)
		if uploadErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed: " + uploadErr.Error()})
			return
		}
		savedImageURLs = append(savedImageURLs, url)
	}

	// Send news data to n8n webhook.
	newsErr := h.sendToN8N(ctx, &team, &input, savedImageURLs)
	if newsErr != nil {
		log.Printf("[NewsCreator] Error sending to n8n: %v", newsErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send news: " + newsErr.Error()})
		return
	}

	// Optionally create a social media post.
	var post *models.Post
	if input.AddSocialPosting {
		created, postErr := h.createPost(ctx, c, &input, savedImageURLs)
		if postErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "News sent but failed to create post: " + postErr.Error()})
			return
		}
		post = created
	}

	resp := gin.H{"message": "News submitted successfully"}
	if post != nil {
		resp["post"] = post
	}
	c.JSON(http.StatusOK, resp)
}

// sendToN8N forwards the news data as multipart/form-data to the configured n8n webhook URL.
func (h *NewsHandler) sendToN8N(ctx context.Context, team *models.Team, input *NewsSubmitInput, imageURLs []string) error {
	log.Printf("[NewsCreator] Sending news to %s (episode %s)", team.NewsCreatorURL, input.EpisodeNumber)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("episode_number", input.EpisodeNumber)
	writer.WriteField("title", input.NewsTagline)
	writer.WriteField("source_url", input.ArticleURL)
	writer.WriteField("notes", input.Shownotes)

	// Attach saved images.
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
			log.Printf("[NewsCreator] Warning: could not read image %s: %v", fullPath, err)
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

	req, err := http.NewRequestWithContext(ctx, "POST", team.NewsCreatorURL, &body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+team.NewsCreatorBearerToken)

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

	log.Printf("[NewsCreator] Success (HTTP %d)", resp.StatusCode)
	return nil
}

// createPost creates a social media post in the database.
func (h *NewsHandler) createPost(ctx context.Context, c *gin.Context, input *NewsSubmitInput, imageURLs []string) (*models.Post, error) {
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

	// Merge any pre-existing imageUrls from the input with saved uploads.
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

// saveUpload saves a single multipart file to disk and returns its URL.
func (h *NewsHandler) saveUpload(fh *multipart.FileHeader) (string, error) {
	ext := filepath.Ext(fh.Filename)
	contentTypes := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".png": "image/png", ".gif": "image/gif", ".webp": "image/webp",
		".mp4": "video/mp4", ".mov": "video/quicktime",
	}
	if _, ok := contentTypes[ext]; !ok {
		return "", fmt.Errorf("unsupported file type %s", ext)
	}
	maxSize := int64(10 * 1024 * 1024) // 10MB for images
	if ext == ".mp4" || ext == ".mov" {
		maxSize = 100 * 1024 * 1024 // 100MB for videos
	}
	if fh.Size > maxSize {
		return "", fmt.Errorf("file too large (max %dMB)", maxSize/(1024*1024))
	}

	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)

	if err := os.MkdirAll(h.UploadDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create upload directory: %w", err)
	}
	dst := filepath.Join(h.UploadDir, filename)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.DB.Uploads().InsertOne(ctx, models.Upload{
		Filename:    filename,
		ContentType: contentTypes[ext],
		Data:        data,
		Size:        int64(len(data)),
		CreatedAt:   time.Now(),
	})

	return "/api/uploads/" + filename, nil
}
