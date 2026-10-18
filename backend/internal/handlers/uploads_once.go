package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/models"
	"socialmedia/internal/uploadstore"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Single-use upload URLs let an MCP client upload a file itself, outside the
// conversation: base64 through a tool call is slow, fills the model's context
// and fails for a video of any size. create_upload_url makes one for a named
// file; the client sends the file there (PUT with the file as the body, or a
// multipart POST with a "file" field, no Authorization header) and the URL is
// spent. The file is stored as a temporary upload of the token's owner, so
// one nothing ends up referencing is freed by the hourly cleanup a day
// later, like the image editor's clips.

const (
	uploadOncePath       = "/api/upload/once/"
	defaultUploadOnceTTL = time.Hour
	maxUploadOnceTTL     = 24 * time.Hour
	maxOnceImageSize     = 30 << 20
)

// onceUploadTypes are the files a single-use URL takes: what the media
// library takes (images, fonts), and the clips and sounds of the image
// editor's videos.
var onceUploadTypes = func() map[string]string {
	types := map[string]string{}
	for ext, ct := range mediaContentTypes {
		types[ext] = ct
	}
	for ext, ct := range tempUploadTypes {
		types[ext] = ct
	}
	return types
}()

// onceUploadLimit is the largest file of a content type.
func onceUploadLimit(ct string) int64 {
	switch {
	case strings.HasPrefix(ct, "video/"):
		return maxClipSize
	case strings.HasPrefix(ct, "audio/"):
		return maxAudioSize
	case isFontContentType(ct):
		return maxFontSize
	default:
		return maxOnceImageSize
	}
}

func hashUploadToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// createUploadToken makes a single-use upload URL for a file named like name
// (its extension sets the type), valid for ttl. It returns the URL to send the
// file to, the URL the file will have once it is there, and the token.
func createUploadToken(ctx context.Context, c *gin.Context, coll *mongo.Collection, name string, ttl time.Duration) (models.UploadToken, string, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	ct, ok := onceUploadTypes[ext]
	if !ok {
		return models.UploadToken{}, "", fmt.Errorf("unsupported file type %q (use jpg, png, gif, webp, mp4, mov, webm, mp3, m4a, aac, wav, ogg, ttf or otf)", ext)
	}
	if ttl <= 0 {
		ttl = defaultUploadOnceTTL
	}
	if ttl > maxUploadOnceTTL {
		ttl = maxUploadOnceTTL
	}
	userID, _ := c.Get("userId")
	uid, err := primitive.ObjectIDFromHex(fmt.Sprint(userID))
	if err != nil {
		return models.UploadToken{}, "", fmt.Errorf("not signed in")
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return models.UploadToken{}, "", err
	}
	token := hex.EncodeToString(buf)
	now := time.Now()
	t := models.UploadToken{
		TokenHash:   hashUploadToken(token),
		UserID:      uid,
		TeamID:      teamIDFrom(c),
		Filename:    fmt.Sprintf("%d%s", now.UnixNano(), ext),
		ContentType: ct,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
	}
	res, err := coll.InsertOne(ctx, t)
	if err != nil {
		return models.UploadToken{}, "", err
	}
	t.ID = res.InsertedID.(primitive.ObjectID)
	return t, token, nil
}

// UploadOnce takes the file for a single-use upload URL: PUT /api/upload/once/
// :token with the file as the body, or POST with a multipart "file" field.
// It needs no other authentication; the token is spent by a successful upload
// (a failed one leaves it usable until it expires).
func (h *PostHandler) UploadOnce(c *gin.Context) {
	token := c.Param("token")
	if len(token) != 64 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown upload URL"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Claiming it first keeps a second request from using it meanwhile.
	var t models.UploadToken
	err := h.DB.UploadTokens().FindOneAndUpdate(ctx,
		bson.M{"tokenHash": hashUploadToken(token), "used": false, "expiresAt": bson.M{"$gt": time.Now()}},
		bson.M{"$set": bson.M{"used": true, "usedAt": time.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&t)
	if err != nil {
		c.JSON(http.StatusGone, gin.H{"error": "This upload URL is unknown, used already or expired; ask for a new one"})
		return
	}
	release := func() {
		h.DB.UploadTokens().UpdateByID(context.Background(), t.ID, bson.M{"$set": bson.M{"used": false}, "$unset": bson.M{"usedAt": ""}}) //nolint:errcheck
	}
	maxSize := onceUploadLimit(t.ContentType)
	data, err := readOnceUpload(c, maxSize)
	if err != nil {
		release()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err = uploadstore.SaveWith(ctx, h.DB, h.UploadDir, t.Filename, t.ContentType, data,
		uploadstore.Meta{Temporary: true, OwnerID: t.UserID.Hex()})
	if errors.Is(err, uploadstore.ErrNeedsBucket) {
		release()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		release()
		log.Printf("single-use upload: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store the file"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": uploadURLPrefix + t.Filename, "contentType": t.ContentType, "size": len(data)})
}

// readOnceUpload reads the file of a single-use upload: a multipart "file"
// field, or else the whole body.
func readOnceUpload(c *gin.Context, maxSize int64) ([]byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize+1<<20)
	tooLarge := fmt.Errorf("file too large (max %d MB)", maxSize>>20)
	var r io.Reader = c.Request.Body
	if strings.HasPrefix(c.ContentType(), "multipart/") {
		fh, err := c.FormFile("file")
		if err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return nil, tooLarge
			}
			return nil, fmt.Errorf("no file field in the form")
		}
		if fh.Size > maxSize {
			return nil, tooLarge
		}
		f, err := fh.Open()
		if err != nil {
			return nil, fmt.Errorf("could not read the file")
		}
		defer f.Close()
		r = f
	}
	data, err := io.ReadAll(io.LimitReader(r, maxSize+1))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, tooLarge
		}
		return nil, fmt.Errorf("could not read the file")
	}
	if int64(len(data)) > maxSize {
		return nil, tooLarge
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("the file is empty")
	}
	return data, nil
}

// toolCreateUploadURL makes a single-use upload URL (create_upload_url).
func (h *MCPHandler) toolCreateUploadURL(c *gin.Context, args map[string]any) (any, bool) {
	name := strings.TrimSpace(strArg(args, "filename"))
	if name == "" {
		return map[string]string{"error": "filename is required (its extension sets the file type)"}, true
	}
	ttl := defaultUploadOnceTTL
	if v, ok := args["expiresInMinutes"].(float64); ok && v > 0 {
		ttl = time.Duration(v) * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t, token, err := createUploadToken(ctx, c, h.DB.UploadTokens(), name, ttl)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	uploadURL := strings.TrimRight(appBaseURL, "/") + uploadOncePath + token
	return map[string]any{
		"uploadUrl":   uploadURL,
		"url":         uploadURLPrefix + t.Filename,
		"contentType": t.ContentType,
		"maxBytes":    onceUploadLimit(t.ContentType),
		"expiresAt":   t.ExpiresAt.UTC().Format(time.RFC3339),
		"howTo": fmt.Sprintf("Send the file once, without an Authorization header: curl -sS --fail-with-body -T %s %q (PUT, the file as the body), or curl -sS --fail-with-body -F file=@%s %q (multipart POST). "+
			"Once it answers {\"url\": …}, use url wherever a tool takes an uploaded file (imageUrls, a design's layer src or audio, add_media, get_image, transcribe). "+
			"An upload nothing uses is deleted after a day.", name, uploadURL, name, uploadURL),
	}, false
}
