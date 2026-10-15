package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Image drafts keep image editor work to be finished later. The editor sends
// the whole edit as JSON: {"template": <the design as a template>, "photo":
// {"src", "crop", "aspectId", "adjustments"}}, photo only when a photo is
// edited. The server checks the outline and records the uploads it uses; the
// editor owns the format. Drafts are the saving user's own.

const (
	maxDraftJSON = 2 * 1024 * 1024
	// A photo's crop is the draft's canvas, so it may be larger than a
	// template's.
	maxDraftSide = 20000
	maxDrafts    = 200
)

// checkImageDraft validates a draft's design and returns its size and the
// uploads it uses (the photo first).
func checkImageDraft(raw string) (width, height int, urls []string, err error) {
	if len(raw) == 0 || len(raw) > maxDraftJSON {
		return 0, 0, nil, fmt.Errorf("draft is empty or larger than %d MB", maxDraftJSON/(1024*1024))
	}
	var d struct {
		Template json.RawMessage `json:"template"`
		Photo    *struct {
			Src string `json:"src"`
		} `json:"photo"`
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return 0, 0, nil, fmt.Errorf("draft is not valid JSON: %v", err)
	}
	width, height, layerURLs, err := checkDesign(string(d.Template), maxDraftJSON, maxDraftSide)
	if err != nil {
		return 0, 0, nil, err
	}
	urls = []string{}
	if d.Photo != nil {
		src := strings.TrimSpace(d.Photo.Src)
		if src == "" || strings.HasPrefix(src, "blob:") || strings.HasPrefix(src, "data:") {
			return 0, 0, nil, fmt.Errorf("the draft's photo must be a stored image")
		}
		if uploadFilename(src) != "" {
			urls = append(urls, src)
		}
	}
	for _, u := range layerURLs {
		if len(urls) == 0 || u != urls[0] {
			urls = append(urls, u)
		}
	}
	return width, height, urls, nil
}

// imageDraftFilter matches the caller's own drafts.
func imageDraftFilter(c *gin.Context) bson.M {
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	return bson.M{"userId": uid}
}

// ListImageDrafts lists the caller's drafts, newest first, without their
// designs (GetImageDraft has them).
func (h *MediaHandler) ListImageDrafts(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	opts := options.Find().
		SetSort(bson.D{{Key: "updatedAt", Value: -1}}).
		SetLimit(maxDrafts).
		SetProjection(bson.M{"design": 0, "designUrls": 0})
	cur, err := h.DB.ImageDrafts().Find(ctx, imageDraftFilter(c), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load drafts"})
		return
	}
	drafts := []models.ImageDraft{}
	if err := cur.All(ctx, &drafts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load drafts"})
		return
	}
	c.JSON(http.StatusOK, drafts)
}

func (h *MediaHandler) GetImageDraft(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	filter := imageDraftFilter(c)
	filter["_id"] = id
	var d models.ImageDraft
	if err := h.DB.ImageDrafts().FindOne(ctx, filter).Decode(&d); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

// CreateImageDraft stores a draft: multipart preview (image), design (JSON)
// and name.
func (h *MediaHandler) CreateImageDraft(c *gin.Context) {
	raw := c.PostForm("design")
	width, height, urls, err := checkImageDraft(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data, ext, err := readTemplatePreview(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		name = "Draft"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	filter := imageDraftFilter(c)
	if n, err := h.DB.ImageDrafts().CountDocuments(ctx, filter); err == nil && n >= maxDrafts {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("You have %d drafts already; delete some first", maxDrafts)})
		return
	}
	url, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("%d%s", time.Now().UnixNano(), ext), mediaContentTypes[ext], data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store the preview"})
		return
	}
	now := time.Now()
	d := models.ImageDraft{
		UserID:     filter["userId"].(primitive.ObjectID),
		TeamID:     teamIDFrom(c),
		Name:       name,
		URL:        url,
		Design:     raw,
		DesignURLs: urls,
		Width:      width,
		Height:     height,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	res, err := h.DB.ImageDrafts().InsertOne(ctx, d)
	if err != nil {
		cleanupUploads(h.DB, h.UploadDir, []string{url})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save the draft"})
		return
	}
	d.ID = res.InsertedID.(primitive.ObjectID)
	d.Design = ""
	c.JSON(http.StatusCreated, d)
}

// UpdateImageDraft replaces a draft's design and preview (as for
// CreateImageDraft); the name changes only when sent. The old preview, and
// images the design no longer uses, are freed.
func (h *MediaHandler) UpdateImageDraft(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
		return
	}
	raw := c.PostForm("design")
	width, height, urls, err := checkImageDraft(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data, ext, err := readTemplatePreview(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	filter := imageDraftFilter(c)
	filter["_id"] = id
	var old models.ImageDraft
	if err := h.DB.ImageDrafts().FindOne(ctx, filter).Decode(&old); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
		return
	}
	url, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("%d%s", time.Now().UnixNano(), ext), mediaContentTypes[ext], data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store the preview"})
		return
	}
	set := bson.M{
		"url":        url,
		"design":     raw,
		"designUrls": urls,
		"width":      width,
		"height":     height,
		"updatedAt":  time.Now(),
	}
	if name, ok := c.GetPostForm("name"); ok && strings.TrimSpace(name) != "" {
		set["name"] = strings.TrimSpace(name)
	}
	var d models.ImageDraft
	err = h.DB.ImageDrafts().FindOneAndUpdate(ctx, filter, bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After).SetProjection(bson.M{"design": 0})).Decode(&d)
	if err != nil {
		cleanupUploads(h.DB, h.UploadDir, []string{url})
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save the draft"})
		return
	}
	cleanupUploads(h.DB, h.UploadDir, append([]string{old.URL}, old.DesignURLs...))
	c.JSON(http.StatusOK, d)
}

// DeleteImageDraft deletes a draft and frees its preview and the images only
// it used.
func (h *MediaHandler) DeleteImageDraft(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	filter := imageDraftFilter(c)
	filter["_id"] = id
	var old models.ImageDraft
	if err := h.DB.ImageDrafts().FindOneAndDelete(ctx, filter).Decode(&old); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
		return
	}
	cleanupUploads(h.DB, h.UploadDir, append([]string{old.URL}, old.DesignURLs...))
	c.JSON(http.StatusOK, gin.H{"message": "Draft deleted"})
}
