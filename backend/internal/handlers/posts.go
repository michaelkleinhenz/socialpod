package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
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
	DB *database.MongoDB
}

type CreatePostInput struct {
	Content      string            `json:"content" binding:"required"`
	FirstComment string            `json:"firstComment,omitempty"`
	Platforms    []models.Platform `json:"platforms" binding:"required"`
	ScheduledAt  string            `json:"scheduledAt" binding:"required"`
	Tags         []string          `json:"tags,omitempty"`
	AccountIDs   map[string]string `json:"accountIds,omitempty"`
	ImageURLs    []string          `json:"imageUrls,omitempty"`
	Status       models.PostStatus `json:"status,omitempty"`
	SuffixIDs    map[string]string `json:"suffixIds,omitempty"`
}

type UpdatePostInput struct {
	Content      *string            `json:"content,omitempty"`
	FirstComment *string            `json:"firstComment,omitempty"`
	Platforms    []models.Platform  `json:"platforms,omitempty"`
	ScheduledAt  *string            `json:"scheduledAt,omitempty"`
	Tags         []string           `json:"tags,omitempty"`
	AccountIDs   map[string]string  `json:"accountIds,omitempty"`
	ImageURLs    []string           `json:"imageUrls,omitempty"`
	Status       *models.PostStatus `json:"status,omitempty"`
	SuffixIDs    map[string]string  `json:"suffixIds"`
}

const (
	blueskyCharLimit   = 300
	instagramCharLimit = 2200
)

func contentLimit(platforms []models.Platform) int {
	limit := instagramCharLimit
	for _, p := range platforms {
		if p == models.PlatformBluesky && blueskyCharLimit < limit {
			limit = blueskyCharLimit
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

	limit := contentLimit(input.Platforms)
	if len([]rune(input.Content)) > limit {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Content exceeds %d character limit", limit)})
		return
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

	post := models.Post{
		UserID:       objID,
		Content:      input.Content,
		FirstComment: input.FirstComment,
		Platforms:    input.Platforms,
		ScheduledAt:  scheduledAt,
		Status:       status,
		Tags:         input.Tags,
		AccountIDs:   input.AccountIDs,
		ImageURLs:    input.ImageURLs,
		SuffixIDs:    input.SuffixIDs,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
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

// saveUpload validates and stores a multipart file in MongoDB, returning its URL.
func (h *PostHandler) saveUpload(ctx context.Context, fh *multipart.FileHeader) (string, error) {
	ext := filepath.Ext(fh.Filename)
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}
	if !allowed[ext] {
		return "", fmt.Errorf("unsupported file type %s", ext)
	}
	if fh.Size > 10*1024*1024 {
		return "", fmt.Errorf("file too large (max 10MB)")
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

	contentTypes := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".png": "image/png", ".gif": "image/gif", ".webp": "image/webp",
	}
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	_, err = h.DB.Uploads().InsertOne(ctx, models.Upload{
		Filename:    filename,
		ContentType: contentTypes[ext],
		Data:        data,
		Size:        int64(len(data)),
		CreatedAt:   time.Now(),
	})
	if err != nil {
		return "", err
	}
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

func (h *PostHandler) ServeImage(c *gin.Context) {
	filename := c.Param("filename")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var upload models.Upload
	err := h.DB.Uploads().FindOne(ctx, bson.M{"filename": filename}).Decode(&upload)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, upload.ContentType, upload.Data)
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
