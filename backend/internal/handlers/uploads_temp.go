package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/backup"
	"socialmedia/internal/database"
	"socialmedia/internal/models"
	"socialmedia/internal/uploadstore"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Temporary uploads: the clips and sounds the image editor puts into a video.
// They belong to nothing when they are uploaded, and usually stay that way:
// the video is rendered into a post (which keeps only the rendered file) and
// the editor is closed. So the editor releases them when it closes, and an
// hourly cleanup frees whatever is left a day after its upload. Either only
// deletes a file nothing references (cleanupUploads), so a clip an image
// draft or a template uses stays, and loses its temporary mark.

// tempUploadTypes are the files the editor takes as working material.
var tempUploadTypes = map[string]string{
	".mp4": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
	".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".aac": "audio/aac",
	".wav": "audio/wav", ".ogg": "audio/ogg",
}

const (
	maxClipSize  = 200 << 20
	maxAudioSize = 50 << 20
	// tempUploadTTL is how long a temporary upload may stay unreferenced: long
	// enough for an editor session, which releases its uploads itself anyway.
	tempUploadTTL = 24 * time.Hour
)

// UploadTemp stores a clip or a sound for the image editor: multipart file.
// The answer carries its url.
func (h *PostHandler) UploadTemp(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	ct, ok := tempUploadTypes[ext]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported file type %s (use a video: mp4, mov, webm, or a sound: mp3, m4a, aac, wav, ogg)", ext)})
		return
	}
	maxSize := int64(maxClipSize)
	if strings.HasPrefix(ct, "audio/") {
		maxSize = maxAudioSize
	}
	if fh.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("file too large (max %d MB)", maxSize>>20)})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read the file"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil || len(data) == 0 || int64(len(data)) > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read the file"})
		return
	}
	userID, _ := c.Get("userId")
	owner, _ := userID.(string)
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err = uploadstore.SaveWith(ctx, h.DB, h.UploadDir, filename, ct, data, uploadstore.Meta{Temporary: true, OwnerID: owner})
	if errors.Is(err, uploadstore.ErrNeedsBucket) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		log.Printf("temporary upload: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store the file"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": uploadURLPrefix + filename, "contentType": ct, "size": len(data)})
}

// ReleaseTemp lets go of temporary uploads: {urls}. Only the caller's own
// temporary uploads are considered, and of those only the ones nothing
// references are freed; the rest stay, no longer temporary.
func (h *PostHandler) ReleaseTemp(c *gin.Context) {
	var input struct {
		URLs []string `json:"urls"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	names := make([]string, 0, len(input.URLs))
	for _, u := range input.URLs {
		if name := uploadFilename(u); name != "" {
			names = append(names, name)
		}
	}
	userID, _ := c.Get("userId")
	owner, _ := userID.(string)
	if len(names) == 0 || owner == "" {
		c.JSON(http.StatusOK, gin.H{"released": 0})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	n, err := releaseTemporaryUploads(ctx, h.DB, h.UploadDir, bson.M{
		"filename": bson.M{"$in": names}, "temporary": true, "ownerId": owner,
	})
	if err != nil {
		log.Printf("releasing temporary uploads: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"released": n})
}

// releaseTemporaryUploads frees the temporary uploads filter matches that
// nothing references, and clears the mark of the ones something does.
func releaseTemporaryUploads(ctx context.Context, db *database.MongoDB, uploadDir string, filter bson.M) (int, error) {
	cursor, err := db.Uploads().Find(ctx, filter, options.Find().SetProjection(bson.M{"filename": 1}).SetLimit(1000))
	if err != nil {
		return 0, err
	}
	var recs []models.Upload
	if err := cursor.All(ctx, &recs); err != nil {
		return 0, err
	}
	if len(recs) == 0 {
		return 0, nil
	}
	urls := make([]string, 0, len(recs))
	for _, r := range recs {
		urls = append(urls, uploadURLPrefix+r.Filename)
	}
	// Referenced files belong to their documents now: no longer temporary.
	if owned := referencedAmong(ctx, db, urls); len(owned) > 0 {
		db.Uploads().UpdateMany(ctx, bson.M{"filename": bson.M{"$in": owned}}, bson.M{"$unset": bson.M{"temporary": "", "ownerId": ""}}) //nolint:errcheck
	}
	freed := cleanupUploads(db, uploadDir, urls)
	return freed, nil
}

// referencedAmong returns the file names of those urls something references.
func referencedAmong(ctx context.Context, db *database.MongoDB, urls []string) []string {
	seen := map[string]bool{}
	for _, ref := range uploadReferences {
		values, err := ref.collection(db).Distinct(ctx, ref.field, bson.M{ref.field: bson.M{"$in": urls}})
		if err != nil {
			return nil
		}
		for _, v := range values {
			if u, ok := v.(string); ok {
				seen[u] = true
			}
		}
	}
	var names []string
	for _, u := range urls {
		if seen[u] {
			names = append(names, uploadFilename(u))
		}
	}
	return names
}

// StartTemporaryUploadCleanup frees, every hour, the temporary uploads older
// than a day that nothing references: clips of an editor that was closed
// without releasing them (a crash, a closed tab).
func StartTemporaryUploadCleanup(db *database.MongoDB, uploadDir string) {
	go func() {
		time.Sleep(time.Minute)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			if !backup.RestoreInProgress() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				n, err := releaseTemporaryUploads(ctx, db, uploadDir, bson.M{
					"temporary": true, "createdAt": bson.M{"$lt": time.Now().Add(-tempUploadTTL)},
				})
				cancel()
				if err != nil {
					log.Printf("temporary upload cleanup: %v", err)
				} else if n > 0 {
					log.Printf("temporary upload cleanup: freed %d unused clips and sounds", n)
				}
			}
			<-ticker.C
		}
	}()
}
