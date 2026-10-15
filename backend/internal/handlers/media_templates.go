package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// Templates are designs for the image editor, kept in the media library: a
// canvas size, a background colour and the editor's layers (texts, and images
// by URL). The editor builds them and renders a preview; the server stores the
// design as JSON, the preview as the entry's file, and the uploads the design's
// images use in templateUrls, which is in uploadReferences.

const (
	maxTemplateJSON   = 512 * 1024
	maxTemplateLayers = 200
	maxTemplateSide   = 4096
	maxTemplatePrev   = 15 * 1024 * 1024
)

// checkTemplate validates a template's JSON and returns the canvas size and
// the uploads its image layers use. Only the outline is checked; the editor
// owns the layer format.
func checkTemplate(raw string) (width, height int, urls []string, err error) {
	return checkDesign(raw, maxTemplateJSON, maxTemplateSide)
}

// checkDesign is checkTemplate with its limits as arguments: an image draft
// on a photo takes the photo's crop as its canvas, which can be larger.
func checkDesign(raw string, maxJSON, maxSide int) (width, height int, urls []string, err error) {
	if len(raw) == 0 || len(raw) > maxJSON {
		return 0, 0, nil, fmt.Errorf("template is empty or larger than %d KB", maxJSON/1024)
	}
	var t struct {
		Width      float64 `json:"width"`
		Height     float64 `json:"height"`
		Background string  `json:"background"`
		Layers     []struct {
			Kind string `json:"kind"`
			Src  string `json:"src"`
		} `json:"layers"`
	}
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return 0, 0, nil, fmt.Errorf("template is not valid JSON: %v", err)
	}
	if t.Width < 1 || t.Height < 1 || t.Width > float64(maxSide) || t.Height > float64(maxSide) {
		return 0, 0, nil, fmt.Errorf("template size must be between 1 and %d pixels a side", maxSide)
	}
	if len(t.Layers) > maxTemplateLayers {
		return 0, 0, nil, fmt.Errorf("a template holds at most %d layers", maxTemplateLayers)
	}
	seen := map[string]bool{}
	urls = []string{}
	for i, l := range t.Layers {
		switch l.Kind {
		case "text", "shape", "paint":
		case "image":
			if strings.TrimSpace(l.Src) == "" {
				return 0, 0, nil, fmt.Errorf("image layer %d has no src", i+1)
			}
			if uploadFilename(l.Src) != "" && !seen[l.Src] {
				seen[l.Src] = true
				urls = append(urls, l.Src)
			}
		default:
			return 0, 0, nil, fmt.Errorf("layer %d has an unknown kind %q", i+1, l.Kind)
		}
	}
	return int(t.Width + 0.5), int(t.Height + 0.5), urls, nil
}

// readTemplatePreview reads the multipart preview image and returns its bytes
// and the extension to store it under.
func readTemplatePreview(c *gin.Context) ([]byte, string, error) {
	fh, err := c.FormFile("preview")
	if err != nil {
		return nil, "", fmt.Errorf("no preview image provided")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, "", fmt.Errorf("could not read the preview")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxTemplatePrev+1))
	if err != nil || len(data) == 0 || len(data) > maxTemplatePrev {
		return nil, "", fmt.Errorf("the preview is empty or larger than %d MB", maxTemplatePrev/(1024*1024))
	}
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return data, ".jpg", nil
	case "image/png":
		return data, ".png", nil
	}
	return nil, "", fmt.Errorf("the preview must be a JPEG or PNG image")
}

// CreateTemplate adds a template: multipart preview (image), template (JSON),
// name, description and comma-separated tags.
func (h *MediaHandler) CreateTemplate(c *gin.Context) {
	raw := c.PostForm("template")
	width, height, urls, err := checkTemplate(raw)
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
		name = "Template"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ct := mediaContentTypes[ext]
	url, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("%d%s", time.Now().UnixNano(), ext), ct, data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store the preview"})
		return
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	item := models.MediaItem{
		UserID:       uid,
		TeamID:       teamIDFrom(c),
		URL:          url,
		Template:     raw,
		TemplateURLs: urls,
		Name:         name,
		Description:  strings.TrimSpace(c.PostForm("description")),
		Tags:         splitTags(c.PostForm("tags")),
		ContentType:  models.MediaTemplateType,
		Width:        width,
		Height:       height,
		Size:         int64(len(data)),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	res, err := h.DB.Media().InsertOne(ctx, item)
	if err != nil {
		cleanupUploads(h.DB, h.UploadDir, []string{url})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save the template"})
		return
	}
	item.ID = res.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, item)
}

// UpdateTemplate replaces a template's design and preview (multipart preview
// and template, as for CreateTemplate); name, description and tags change only
// when sent. The old preview, and images the design no longer uses, are freed.
func (h *MediaHandler) UpdateTemplate(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid media ID"})
		return
	}
	raw := c.PostForm("template")
	width, height, urls, err := checkTemplate(raw)
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

	filter := mcpScopeFilter(c)
	filter["_id"] = id
	filter["contentType"] = models.MediaTemplateType
	var old models.MediaItem
	if err := h.DB.Media().FindOne(ctx, filter).Decode(&old); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	url, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("%d%s", time.Now().UnixNano(), ext), mediaContentTypes[ext], data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store the preview"})
		return
	}
	set := bson.M{
		"url":          url,
		"template":     raw,
		"templateUrls": urls,
		"width":        width,
		"height":       height,
		"size":         int64(len(data)),
		"updatedAt":    time.Now(),
	}
	if name, ok := c.GetPostForm("name"); ok && strings.TrimSpace(name) != "" {
		set["name"] = strings.TrimSpace(name)
	}
	if desc, ok := c.GetPostForm("description"); ok {
		set["description"] = strings.TrimSpace(desc)
	}
	if tags, ok := c.GetPostForm("tags"); ok {
		set["tags"] = splitTags(tags)
	}
	var item models.MediaItem
	err = h.DB.Media().FindOneAndUpdate(ctx, filter, bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&item)
	if err != nil {
		cleanupUploads(h.DB, h.UploadDir, []string{url})
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save the template"})
		return
	}
	cleanupUploads(h.DB, h.UploadDir, append([]string{old.URL}, old.TemplateURLs...))
	c.JSON(http.StatusOK, item)
}
