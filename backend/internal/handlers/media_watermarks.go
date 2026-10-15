package handlers

import (
	"bytes"
	"context"
	"image"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Watermarks are media library images flagged watermark: true. They used to
// live in a collection of their own; MigrateWatermarksToMedia moves those
// documents into the library under their old IDs, so every watermarkId kept
// on teams, convention queues and in templates still resolves.

// findMediaItem loads a library entry by ID, unscoped: the callers hold an ID
// taken from the team's own settings or queue.
func findMediaItem(ctx context.Context, db *database.MongoDB, id primitive.ObjectID) (models.MediaItem, error) {
	var item models.MediaItem
	err := db.Media().FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	return item, err
}

// listWatermarks returns the scope's watermarks, oldest first like the old
// collection's natural order.
func listWatermarks(ctx context.Context, db *database.MongoDB, scope bson.M) ([]models.MediaItem, error) {
	filter := bson.M{"watermark": true}
	for k, v := range scope {
		filter[k] = v
	}
	cursor, err := db.Media().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	items := []models.MediaItem{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// deleteWatermark removes a watermark from the library and frees its file.
func deleteWatermark(ctx context.Context, db *database.MongoDB, uploadDir string, scope bson.M, id primitive.ObjectID) error {
	filter := bson.M{"_id": id, "watermark": true}
	for k, v := range scope {
		filter[k] = v
	}
	urls, err := deleteOneAndCollectUploadURLs(ctx, db.Media(), filter)
	if err != nil {
		return err
	}
	cleanupUploads(db, uploadDir, urls)
	return nil
}

// MigrateWatermarksToMedia moves the documents of the former watermarks
// collection into the media library as watermark images, keeping their IDs,
// and drops the collection once it is empty. It is idempotent: an entry
// already moved is only flagged again. It runs on startup and after a restore,
// which may bring the old collection back.
func MigrateWatermarksToMedia(ctx context.Context, db *database.MongoDB, uploadDir string) (int, error) {
	cursor, err := db.Watermarks().Find(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	var legacy []models.Watermark
	if err := cursor.All(ctx, &legacy); err != nil {
		return 0, err
	}
	moved := 0
	for _, wm := range legacy {
		item := models.MediaItem{
			ID:          wm.ID,
			UserID:      wm.UserID,
			TeamID:      wm.TeamID,
			URL:         wm.URL,
			Name:        strings.TrimSpace(wm.Name),
			Description: wm.Description,
			Watermark:   true,
			ContentType: mediaContentTypes[strings.ToLower(filepath.Ext(wm.URL))],
			CreatedAt:   wm.CreatedAt,
			UpdatedAt:   wm.CreatedAt,
		}
		if item.Name == "" {
			item.Name = strings.TrimSuffix(wm.Filename, filepath.Ext(wm.Filename))
		}
		if item.Name == "" {
			item.Name = "Watermark"
		}
		if !strings.HasPrefix(item.ContentType, "image/") {
			item.ContentType = "image/png"
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt, item.UpdatedAt = time.Now(), time.Now()
		}
		if data, err := readImageBytes(ctx, db, uploadDir, wm.URL); err == nil {
			item.Size = int64(len(data))
			if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
				item.Width, item.Height = cfg.Width, cfg.Height
			}
		}
		if _, err := db.Media().InsertOne(ctx, item); err != nil {
			if !mongo.IsDuplicateKeyError(err) {
				return moved, err
			}
			if _, err := db.Media().UpdateByID(ctx, wm.ID, bson.M{"$set": bson.M{"watermark": true}}); err != nil {
				return moved, err
			}
		}
		if _, err := db.Watermarks().DeleteOne(ctx, bson.M{"_id": wm.ID}); err != nil {
			return moved, err
		}
		moved++
	}
	if n, err := db.Watermarks().CountDocuments(ctx, bson.M{}); err == nil && n == 0 {
		_ = db.Watermarks().Drop(ctx)
	}
	return moved, nil
}

// The /api/watermarks endpoints remain for API clients (the n8n node): they
// are the media library narrowed to watermarks.

func (h *MediaHandler) ListWatermarks(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	items, err := listWatermarks(ctx, h.DB, mcpScopeFilter(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch watermarks"})
		return
	}
	c.JSON(http.StatusOK, items)
}

// UploadWatermark adds an image to the library as a watermark: multipart
// image, optional name and description.
func (h *MediaHandler) UploadWatermark(c *gin.Context) {
	fh, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file provided"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read the file"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 16*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read the file"})
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !strings.HasPrefix(mediaContentTypes[ext], "image/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A watermark must be a JPEG, PNG, GIF or WebP image"})
		return
	}
	name := c.PostForm("name")
	if name == "" {
		name = strings.TrimSuffix(fh.Filename, filepath.Ext(fh.Filename))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	item, err := addMediaItem(ctx, c, h.DB, h.UploadDir, data, ext, name, c.PostForm("description"), nil)
	if err == nil {
		item, err = markWatermark(ctx, h.DB, item)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

// markWatermark flags a freshly added image as a watermark.
func markWatermark(ctx context.Context, db *database.MongoDB, item models.MediaItem) (models.MediaItem, error) {
	if _, err := db.Media().UpdateByID(ctx, item.ID, bson.M{"$set": bson.M{"watermark": true}}); err != nil {
		return item, err
	}
	item.Watermark = true
	return item, nil
}

// UpdateWatermark renames a watermark or changes its description.
func (h *MediaHandler) UpdateWatermark(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
		return
	}
	var input struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	scope := mcpScopeFilter(c)
	scope["watermark"] = true
	item, err := updateMediaItem(ctx, h.DB, scope, id, mediaUpdate{Name: input.Name, Description: input.Description})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Watermark not found"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *MediaHandler) DeleteWatermark(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := deleteWatermark(ctx, h.DB, h.UploadDir, mcpScopeFilter(c), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Watermark not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Watermark deleted"})
}
