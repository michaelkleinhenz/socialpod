package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
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

type PostHandler struct {
	DB        *database.MongoDB
	UploadDir string
}

type EpisodeNewsInput struct {
	EpisodeNumber  string `json:"episodeNumber"`
	Title          string `json:"title"`
	AdditionalText string `json:"additionalText,omitempty"`
	BGGLink        string `json:"bggLink,omitempty"`
}

type CreatePostInput struct {
	Content          string            `json:"content"`
	PostType         models.PostType   `json:"postType,omitempty"`
	FirstComment     string            `json:"firstComment,omitempty"`
	Platforms        []models.Platform `json:"platforms" binding:"required"`
	ScheduledAt      string            `json:"scheduledAt" binding:"required"`
	Tags             []string          `json:"tags,omitempty"`
	AccountIDs       map[string]string `json:"accountIds,omitempty"`
	ImageURLs        []string          `json:"imageUrls,omitempty"`
	Status           models.PostStatus `json:"status,omitempty"`
	SuffixIDs        map[string]string `json:"suffixIds,omitempty"`
	ContentOverrides map[string]string `json:"contentOverrides,omitempty"`
	EpisodeNews      *EpisodeNewsInput `json:"episodeNews,omitempty"`
}

type UpdatePostInput struct {
	Content          *string            `json:"content,omitempty"`
	PostType         *models.PostType   `json:"postType,omitempty"`
	FirstComment     *string            `json:"firstComment,omitempty"`
	Platforms        []models.Platform  `json:"platforms,omitempty"`
	ScheduledAt      *string            `json:"scheduledAt,omitempty"`
	Tags             []string           `json:"tags,omitempty"`
	AccountIDs       map[string]string  `json:"accountIds,omitempty"`
	ImageURLs        []string           `json:"imageUrls,omitempty"`
	Status           *models.PostStatus `json:"status,omitempty"`
	SuffixIDs        map[string]string  `json:"suffixIds"`
	ContentOverrides map[string]string  `json:"contentOverrides"`
}

const (
	blueskyCharLimit   = 300
	threadsCharLimit   = 500
	instagramCharLimit = 2200
	linkedInCharLimit  = 3000
)

func contentLimit(platforms []models.Platform) int {
	limit := linkedInCharLimit
	for _, p := range platforms {
		var platformLimit int
		switch p {
		case models.PlatformBluesky:
			platformLimit = blueskyCharLimit
		case models.PlatformThreads:
			platformLimit = threadsCharLimit
		case models.PlatformInstagram:
			platformLimit = instagramCharLimit
		default:
			continue
		}
		if platformLimit < limit {
			limit = platformLimit
		}
	}
	return limit
}

// postFilter returns a bson filter scoped to the user's team (if any) or user.
func postFilter(c *gin.Context) bson.M {
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		return bson.M{"teamId": tid}
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	return bson.M{"userId": uid}
}

func (h *PostHandler) Create(c *gin.Context) {
	// Accept multipart/form-data: "data" field holds the JSON payload,
	// optional "images" fields hold image files to upload in the same request.
	var input CreatePostInput
	if err := json.Unmarshal([]byte(c.PostForm("data")), &input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post data: " + err.Error()})
		return
	}

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	if input.Content != "" {
		limit := contentLimit(input.Platforms)
		if len([]rune(input.Content)) > limit {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Content exceeds %d character limit", limit)})
			return
		}
	}

	scheduledAt, err := time.Parse(time.RFC3339, input.ScheduledAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid scheduledAt format, use RFC3339"})
		return
	}

	status := models.PostStatusScheduled
	if input.Status != "" {
		status = input.Status
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Upload any attached image files and append their URLs to the post.
	if form, err := c.MultipartForm(); err == nil {
		for _, fh := range form.File["images"] {
			url, uploadErr := h.saveUpload(ctx, fh)
			if uploadErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed: " + uploadErr.Error()})
				return
			}
			input.ImageURLs = append(input.ImageURLs, url)
		}
	}

	postType := input.PostType
	if postType == "" {
		postType = models.PostTypePost
	}

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
		ImageURLs:        input.ImageURLs,
		SuffixIDs:        input.SuffixIDs,
		ContentOverrides: input.ContentOverrides,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if input.EpisodeNews != nil {
		post.EpisodeNews = &models.EpisodeNews{
			Enabled:        true,
			EpisodeNumber:  input.EpisodeNews.EpisodeNumber,
			Title:          input.EpisodeNews.Title,
			AdditionalText: input.EpisodeNews.AdditionalText,
			BGGLink:        input.EpisodeNews.BGGLink,
		}
	}

	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		post.TeamID = &tid
	}

	result, err := h.DB.Posts().InsertOne(ctx, post)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create post"})
		return
	}

	post.ID = result.InsertedID.(primitive.ObjectID)

	if post.EpisodeNews != nil && post.EpisodeNews.Enabled && post.TeamID != nil {
		h.sendEpisodeNews(ctx, &post)
	}

	c.JSON(http.StatusCreated, post)
}

func (h *PostHandler) List(c *gin.Context) {
	filter := postFilter(c)

	// Optional date range filtering
	if start := c.Query("start"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			if filter["scheduledAt"] == nil {
				filter["scheduledAt"] = bson.M{}
			}
			filter["scheduledAt"].(bson.M)["$gte"] = t
		}
	}
	if end := c.Query("end"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			if filter["scheduledAt"] == nil {
				filter["scheduledAt"] = bson.M{}
			}
			filter["scheduledAt"].(bson.M)["$lte"] = t
		}
	}
	if status := c.Query("status"); status != "" {
		filter["status"] = status
	}
	if platform := c.Query("platform"); platform != "" {
		filter["platforms"] = platform
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "scheduledAt", Value: 1}})
	cursor, err := h.DB.Posts().Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch posts"})
		return
	}
	defer cursor.Close(ctx)

	var posts []models.Post
	if err := cursor.All(ctx, &posts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode posts"})
		return
	}

	if posts == nil {
		posts = []models.Post{}
	}

	c.JSON(http.StatusOK, posts)
}

func (h *PostHandler) Get(c *gin.Context) {
	postID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post ID"})
		return
	}

	filter := postFilter(c)
	filter["_id"] = postID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var post models.Post
	err = h.DB.Posts().FindOne(ctx, filter).Decode(&post)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) Update(c *gin.Context) {
	postID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post ID"})
		return
	}

	var input UpdatePostInput
	if err := json.Unmarshal([]byte(c.PostForm("data")), &input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post data: " + err.Error()})
		return
	}

	// Validate content length against platform limits
	if input.Content != nil || input.Platforms != nil {
		platforms := input.Platforms
		if platforms == nil {
			// Fetch existing post to get current platforms
			ctx0, cancel0 := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel0()
			pf := postFilter(c)
			pf["_id"] = postID
			var existing models.Post
			if err := h.DB.Posts().FindOne(ctx0, pf).Decode(&existing); err == nil {
				platforms = existing.Platforms
			}
		}
		if input.Content != nil && platforms != nil {
			limit := contentLimit(platforms)
			if len([]rune(*input.Content)) > limit {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Content exceeds %d character limit", limit)})
				return
			}
		}
	}

	filter := postFilter(c)
	filter["_id"] = postID

	// Upload any attached image files and append their URLs.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if form, err := c.MultipartForm(); err == nil {
		for _, fh := range form.File["images"] {
			url, uploadErr := h.saveUpload(ctx, fh)
			if uploadErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed: " + uploadErr.Error()})
				return
			}
			input.ImageURLs = append(input.ImageURLs, url)
		}
	}

	update := bson.M{"updatedAt": time.Now()}

	if input.Content != nil {
		update["content"] = *input.Content
	}
	if input.PostType != nil {
		update["postType"] = *input.PostType
	}
	if input.FirstComment != nil {
		update["firstComment"] = *input.FirstComment
	}
	if input.Platforms != nil {
		update["platforms"] = input.Platforms
	}
	if input.ScheduledAt != nil {
		if t, err := time.Parse(time.RFC3339, *input.ScheduledAt); err == nil {
			update["scheduledAt"] = t
		}
	}
	if input.Tags != nil {
		update["tags"] = input.Tags
	}
	if input.AccountIDs != nil {
		update["accountIds"] = input.AccountIDs
	}
	if input.ImageURLs != nil {
		update["imageUrls"] = input.ImageURLs
	}
	if input.Status != nil {
		update["status"] = *input.Status
	}
	if input.SuffixIDs != nil {
		update["suffixIds"] = input.SuffixIDs
	}
	// ContentOverrides uses explicit null-check via the json tag (no omitempty) so
	// an empty map from the client clears all overrides, while absence keeps existing.
	if input.ContentOverrides != nil {
		update["contentOverrides"] = input.ContentOverrides
	}

	result, err := h.DB.Posts().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	var post models.Post
	h.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) Delete(c *gin.Context) {
	postID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post ID"})
		return
	}

	filter := postFilter(c)
	filter["_id"] = postID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.DB.Posts().DeleteOne(ctx, filter)
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Post deleted"})
}

// saveUpload validates and stores a multipart file on disk, returning its URL.
func (h *PostHandler) saveUpload(ctx context.Context, fh *multipart.FileHeader) (string, error) {
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
	return h.storeFile(filename, contentTypes[ext], data)
}

// storeFile writes file data to disk and persists it in MongoDB so
// uploads survive ephemeral filesystem restarts (e.g. Railway).
func (h *PostHandler) storeFile(filename, contentType string, data []byte) (string, error) {
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
		ContentType: contentType,
		Data:        data,
		Size:        int64(len(data)),
		CreatedAt:   time.Now(),
	})
	return "/api/uploads/" + filename, nil
}

// UploadImage handles the standalone POST /api/upload endpoint (used by
// the Adobe Express onPublish callback to upload design exports).
func (h *PostHandler) UploadImage(c *gin.Context) {
	_, fh, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file provided"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url, err := h.saveUpload(ctx, fh)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url, "filename": filepath.Base(url)})
}

// safeURLClient is an HTTP client with timeouts used for user-supplied URLs.
var safeURLClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return validateURL(req.URL)
	},
}

// validateURL checks that a URL is safe to fetch server-side:
// only http/https schemes, and the resolved IP is not a private/reserved range.
func validateURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return errors.New("empty host")
	}

	// Resolve all IPs for the host and reject private/reserved ranges.
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %w", err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("URL resolves to a private address")
		}
	}
	return nil
}

// isPrivateIP returns true for loopback, link-local, and RFC-1918 ranges.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network string
	}{
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"127.0.0.0/8"},
		{"169.254.0.0/16"},
		{"0.0.0.0/8"},
		{"::1/128"},
		{"fc00::/7"},
		{"fe80::/10"},
	}
	for _, r := range privateRanges {
		_, cidr, _ := net.ParseCIDR(r.network)
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// UploadFromURL downloads an image/video from an external URL and stores it.
// Used by the Adobe Express integration to bypass CORS restrictions.
func (h *PostHandler) UploadFromURL(c *gin.Context) {
	var input struct {
		URL string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}

	// Validate URL before fetching to prevent SSRF.
	parsedURL, err := url.Parse(input.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid URL"})
		return
	}
	if err := validateURL(parsedURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Retry download up to 3 times — the asset may not be ready on S3 yet.
	var data []byte
	var hdrCT string
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}
		resp, err := safeURLClient.Get(input.URL)
		if err != nil {
			if attempt == 2 {
				c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to download: " + err.Error()})
				return
			}
			continue
		}
		hdrCT = resp.Header.Get("Content-Type")
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != 200 {
			if attempt == 2 {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Download failed: status %d, content-type %s", resp.StatusCode, hdrCT)})
				return
			}
			continue
		}
		data = body
		break
	}

	if len(data) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Downloaded file is empty"})
		return
	}

	// Detect content type from file magic bytes — the S3 Content-Type
	// header is often wrong (application/octet-stream or video/mp4 for PNGs).
	ct := http.DetectContentType(data)
	log.Printf("UploadFromURL: downloaded %d bytes, detected=%q, header=%q", len(data), ct, hdrCT)

	extMap := map[string]string{
		"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif",
		"image/webp": ".webp", "video/mp4": ".mp4",
	}

	// Check magic bytes first: PNG (\x89PNG), JPEG (\xff\xd8), GIF (GIF8)
	ext := ".png"
	if len(data) >= 4 {
		switch {
		case data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
			ct = "image/png"
			ext = ".png"
		case data[0] == 0xFF && data[1] == 0xD8:
			ct = "image/jpeg"
			ext = ".jpg"
		case data[0] == 'G' && data[1] == 'I' && data[2] == 'F' && data[3] == '8':
			ct = "image/gif"
			ext = ".gif"
		case data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' && len(data) >= 12 &&
			data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P':
			ct = "image/webp"
			ext = ".webp"
		default:
			// If magic bytes don't match a known image format, use DetectContentType result.
			if e, ok := extMap[ct]; ok {
				ext = e
			} else {
				// Default to PNG for Adobe Express designs.
				ct = "image/png"
				ext = ".png"
			}
		}
	}

	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	url, storeErr := h.storeFile(filename, ct, data)
	if storeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url, "filename": filename})
}

func (h *PostHandler) ServeImage(c *gin.Context) {
	filename := c.Param("filename")
	diskPath := filepath.Join(h.UploadDir, filename)

	// Try disk cache first.
	if _, err := os.Stat(diskPath); err == nil {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.File(diskPath)
		return
	}

	// Serve from MongoDB (durable store).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var upload models.Upload
	err := h.DB.Uploads().FindOne(ctx, bson.M{"filename": filename}).Decode(&upload)
	if err != nil {
		log.Printf("ServeImage: MongoDB lookup failed for %s: %v", filename, err)
		c.Status(http.StatusNotFound)
		return
	}
	if len(upload.Data) == 0 {
		log.Printf("ServeImage: MongoDB record found for %s but Data is empty (size=%d, contentType=%s)", filename, upload.Size, upload.ContentType)
		c.Status(http.StatusNotFound)
		return
	}

	log.Printf("ServeImage: serving %s from MongoDB (%d bytes, %s)", filename, len(upload.Data), upload.ContentType)

	// Re-cache to disk so subsequent requests bypass MongoDB.
	os.MkdirAll(h.UploadDir, 0o755)
	os.WriteFile(diskPath, upload.Data, 0o644)

	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, upload.ContentType, upload.Data)
}

func (h *PostHandler) Retry(c *gin.Context) {
	postID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post ID"})
		return
	}

	filter := postFilter(c)
	filter["_id"] = postID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var post models.Post
	if err := h.DB.Posts().FindOne(ctx, filter).Decode(&post); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	if post.Status != models.PostStatusFailed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only failed posts can be retried"})
		return
	}

	_, err = h.DB.Posts().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{
			"status":      models.PostStatusScheduled,
			"scheduledAt": time.Now(),
			"results":     nil,
			"updatedAt":   time.Now(),
		},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retry post"})
		return
	}

	h.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) suffixContent(ctx context.Context, suffixIDStr string, teamID *primitive.ObjectID) string {
	if suffixIDStr == "" {
		return ""
	}
	oid, err := primitive.ObjectIDFromHex(suffixIDStr)
	if err != nil {
		return ""
	}
	filter := bson.M{"_id": oid}
	if teamID != nil {
		filter["teamId"] = teamID
	}
	var suffix models.Suffix
	if err := h.DB.Suffixes().FindOne(ctx, filter).Decode(&suffix); err != nil {
		return ""
	}
	return suffix.Content
}

func (h *PostHandler) sendEpisodeNews(ctx context.Context, post *models.Post) {
	log.Printf("[EpisodeNews] Sending episode news for post %s (episode %s)", post.ID.Hex(), post.EpisodeNews.EpisodeNumber)

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": post.TeamID}).Decode(&team); err != nil {
		log.Printf("[EpisodeNews] Error: team not found for post %s: %v", post.ID.Hex(), err)
		h.updateNewsResult(ctx, post.ID, false, "Team not found")
		return
	}

	pluginEnabled := false
	for _, p := range team.EnabledPlugins {
		if p == "episode_news" {
			pluginEnabled = true
			break
		}
	}
	if !pluginEnabled {
		log.Printf("[EpisodeNews] Skipped: episode_news plugin not enabled for team %s", team.ID.Hex())
		h.updateNewsResult(ctx, post.ID, false, "Episode news plugin not enabled for this team")
		return
	}

	if team.EpisodeNewsURL == "" || team.EpisodeNewsBearerToken == "" {
		log.Printf("[EpisodeNews] Error: URL or bearer token not configured for team %s", team.ID.Hex())
		h.updateNewsResult(ctx, post.ID, false, "Episode news URL or bearer token not configured")
		return
	}

	log.Printf("[EpisodeNews] Target URL: %s", team.EpisodeNewsURL)

	platformContent := func(platform string) string {
		if post.ContentOverrides != nil {
			if override, ok := post.ContentOverrides[platform]; ok && override != "" {
				return override
			}
		}
		return post.Content
	}

	instagramText := platformContent("instagram")
	bskyText := platformContent("bluesky")

	if post.SuffixIDs != nil {
		if s := h.suffixContent(ctx, post.SuffixIDs["instagram"], post.TeamID); s != "" {
			instagramText = instagramText + "\n" + s
		}
		if s := h.suffixContent(ctx, post.SuffixIDs["bluesky"], post.TeamID); s != "" {
			bskyText = bskyText + "\n" + s
		}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("episodeNumber", post.EpisodeNews.EpisodeNumber)
	writer.WriteField("title", post.EpisodeNews.Title)
	writer.WriteField("link", post.EpisodeNews.BGGLink)
	writer.WriteField("additionalText", post.EpisodeNews.AdditionalText)
	writer.WriteField("instagramText", instagramText)
	writer.WriteField("bskyText", bskyText)
	writer.WriteField("postingDate", post.ScheduledAt.Format(time.RFC3339))

	if len(post.ImageURLs) > 0 {
		imgPath := post.ImageURLs[0]
		if imgPath != "" && imgPath[0] == '/' {
			imgPath = imgPath[1:]
		}
		fullPath := filepath.Join(h.UploadDir, filepath.Base(imgPath))
		if fileData, err := os.ReadFile(fullPath); err == nil {
			ext := filepath.Ext(fullPath)
			mimeType := mime.TypeByExtension(ext)
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			part, _ := writer.CreateFormFile("image", filepath.Base(fullPath))
			part.Write(fileData)
		}
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", team.EpisodeNewsURL, &body)
	if err != nil {
		log.Printf("[EpisodeNews] Error: failed to create request for post %s: %v", post.ID.Hex(), err)
		h.updateNewsResult(ctx, post.ID, false, "Failed to create request: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+team.EpisodeNewsBearerToken)

	// DEBUG: log full request details including headers
	log.Printf("[EpisodeNews] === DEBUG REQUEST for post %s ===", post.ID.Hex())
	log.Printf("[EpisodeNews] Method: %s", req.Method)
	log.Printf("[EpisodeNews] URL: %s", req.URL.String())
	for name, values := range req.Header {
		for _, v := range values {
			log.Printf("[EpisodeNews] Header: %s: %s", name, v)
		}
	}
	log.Printf("[EpisodeNews] Body size: %d bytes", body.Len())
	log.Printf("[EpisodeNews] === END DEBUG REQUEST ===")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[EpisodeNews] Error: request failed for post %s: %v", post.ID.Hex(), err)
		h.updateNewsResult(ctx, post.ID, false, "Request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[EpisodeNews] Success: post %s sent successfully (HTTP %d)", post.ID.Hex(), resp.StatusCode)
		h.updateNewsResult(ctx, post.ID, true, "")
	} else {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Printf("[EpisodeNews] Error: post %s received HTTP %d: %s", post.ID.Hex(), resp.StatusCode, string(respBody))
		h.updateNewsResult(ctx, post.ID, false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody)))
	}
}

func (h *PostHandler) updateNewsResult(ctx context.Context, postID primitive.ObjectID, success bool, errMsg string) {
	result := models.NewsResult{
		Success: success,
		SentAt:  time.Now(),
		Error:   errMsg,
	}
	h.DB.Posts().UpdateOne(ctx, bson.M{"_id": postID}, bson.M{
		"$set": bson.M{
			"episodeNews.result": result,
			"updatedAt":          time.Now(),
		},
		"$push": bson.M{
			"episodeNews.resultHistory": result,
		},
	})
}

func (h *PostHandler) RetryEpisodeNews(c *gin.Context) {
	postID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post ID"})
		return
	}

	filter := postFilter(c)
	filter["_id"] = postID

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var post models.Post
	if err := h.DB.Posts().FindOne(ctx, filter).Decode(&post); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	if post.EpisodeNews == nil || !post.EpisodeNews.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Post has no episode news configured"})
		return
	}

	h.sendEpisodeNews(ctx, &post)

	h.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) Reschedule(c *gin.Context) {
	postID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post ID"})
		return
	}

	var input struct {
		ScheduledAt string `json:"scheduledAt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scheduledAt, err := time.Parse(time.RFC3339, input.ScheduledAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format"})
		return
	}

	filter := postFilter(c)
	filter["_id"] = postID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.DB.Posts().UpdateOne(ctx,
		filter,
		bson.M{"$set": bson.M{"scheduledAt": scheduledAt, "updatedAt": time.Now()}},
	)
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	var post models.Post
	h.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	c.JSON(http.StatusOK, post)
}
