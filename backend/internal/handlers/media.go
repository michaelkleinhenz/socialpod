package handlers

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"
	"socialmedia/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MediaHandler serves the team media library: uploaded material (photos,
// artwork, logos, backgrounds, fonts) with a name, a description and tags,
// which people and agents build post images from; compose_image draws text
// in a library font (fontMediaId). Files are ordinary uploads; a
// library entry only references one, and media.url is in uploadReferences so
// cleanup and the sweep keep files the library still holds.
type MediaHandler struct {
	DB        *database.MongoDB
	UploadDir string
}

var mediaContentTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".gif": "image/gif", ".webp": "image/webp",
	".mp4": "video/mp4", ".mov": "video/quicktime",
	".ttf": "font/ttf", ".otf": "font/otf",
}

func isFontContentType(ct string) bool { return strings.HasPrefix(ct, "font/") }

// mediaKindPrefixes maps a list filter kind to the content type prefix.
var mediaKindPrefixes = map[string]string{"image": "image/", "video": "video/", "font": "font/", "color": "color/", "template": "template/"}

var hexColorPattern = regexp.MustCompile(`^#?([0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$`)

// normalizeColor turns #rgb, #rrggbb or #rrggbbaa (the # optional) into
// lower-case #rrggbb, or #rrggbbaa when it is not fully opaque.
func normalizeColor(s string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(s))
	if !hexColorPattern.MatchString(h) {
		return "", fmt.Errorf("%q is not a colour (use #rrggbb or #rrggbbaa)", s)
	}
	h = strings.TrimPrefix(h, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) == 8 && strings.HasSuffix(h, "ff") {
		h = h[:6]
	}
	return "#" + h, nil
}

// addMediaColor records a colour in the library. The description is meant to
// say how and where to use it.
func addMediaColor(ctx context.Context, c *gin.Context, db *database.MongoDB, hex, name, description string, tags []string) (models.MediaItem, error) {
	hex, err := normalizeColor(hex)
	if err != nil {
		return models.MediaItem{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = hex
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	item := models.MediaItem{
		UserID:      uid,
		TeamID:      teamIDFrom(c),
		Color:       hex,
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		Tags:        normalizeTags(tags),
		ContentType: models.MediaColorType,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	res, err := db.Media().InsertOne(ctx, item)
	if err != nil {
		return models.MediaItem{}, err
	}
	item.ID = res.InsertedID.(primitive.ObjectID)
	return item, nil
}

// normalizeTags lower-cases, trims and de-duplicates tags.
func normalizeTags(tags []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#")))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func splitTags(s string) []string {
	return normalizeTags(strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }))
}

// mediaListFilter builds the scoped query for a library search: words of
// query must all appear in the name, description, tags or colour value; every
// tag given must be present; kind (image, video, font, color, template; empty = all)
// limits the content type.
func mediaListFilter(scope bson.M, query string, tags []string, kind string) bson.M {
	filter := bson.M{}
	for k, v := range scope {
		filter[k] = v
	}
	if prefix, ok := mediaKindPrefixes[strings.ToLower(strings.TrimSpace(kind))]; ok {
		filter["contentType"] = primitive.Regex{Pattern: "^" + regexp.QuoteMeta(prefix)}
	}
	var and []bson.M
	for _, w := range strings.Fields(query) {
		re := primitive.Regex{Pattern: regexp.QuoteMeta(w), Options: "i"}
		and = append(and, bson.M{"$or": []bson.M{
			{"name": re}, {"description": re}, {"tags": re}, {"color": re},
		}})
	}
	if tags = normalizeTags(tags); len(tags) > 0 {
		and = append(and, bson.M{"tags": bson.M{"$all": tags}})
	}
	if len(and) > 0 {
		filter["$and"] = and
	}
	return filter
}

func listMedia(ctx context.Context, db *database.MongoDB, scope bson.M, query string, tags []string, kind string, limit int) ([]models.MediaItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	opts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(int64(limit))
	cursor, err := db.Media().Find(ctx, mediaListFilter(scope, query, tags, kind), opts)
	if err != nil {
		return nil, err
	}
	items := []models.MediaItem{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// addMediaItem stores data as an upload and records it in the library.
func addMediaItem(ctx context.Context, c *gin.Context, db *database.MongoDB, uploadDir string, data []byte, ext, name, description string, tags []string) (models.MediaItem, error) {
	ext = strings.ToLower(ext)
	ct, ok := mediaContentTypes[ext]
	if !ok {
		return models.MediaItem{}, fmt.Errorf("unsupported file type %s", ext)
	}
	if isFontContentType(ct) {
		// It must parse and draw Latin text with umlauts. The contents
		// decide .ttf or .otf.
		var err error
		if ext, ct, err = checkFontUpload("font"+ext, data); err != nil {
			return models.MediaItem{}, err
		}
	}
	maxSize := 15 * 1024 * 1024
	if strings.HasPrefix(ct, "video/") {
		maxSize = 100 * 1024 * 1024
	}
	if len(data) == 0 || len(data) > maxSize {
		return models.MediaItem{}, fmt.Errorf("file is empty or larger than %d MB", maxSize/(1024*1024))
	}
	url, err := storeUploadBytes(ctx, db, uploadDir, fmt.Sprintf("%d%s", time.Now().UnixNano(), ext), ct, data)
	if err != nil {
		return models.MediaItem{}, err
	}
	return recordMediaItem(ctx, c, db, url, data, ct, name, description, tags)
}

// recordMediaItem adds an already stored upload to the library.
func recordMediaItem(ctx context.Context, c *gin.Context, db *database.MongoDB, url string, data []byte, ct, name, description string, tags []string) (models.MediaItem, error) {
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(filepath.Base(url), filepath.Ext(url))
	}
	item := models.MediaItem{
		UserID:      uid,
		TeamID:      teamIDFrom(c),
		URL:         url,
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		Tags:        normalizeTags(tags),
		ContentType: ct,
		Size:        int64(len(data)),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if strings.HasPrefix(ct, "image/") {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			item.Width, item.Height = cfg.Width, cfg.Height
		}
	}
	res, err := db.Media().InsertOne(ctx, item)
	if err != nil {
		return models.MediaItem{}, err
	}
	item.ID = res.InsertedID.(primitive.ObjectID)
	return item, nil
}

// errNotAColor is returned when a colour is set on an entry that is a file.
var errNotAColor = fmt.Errorf("only a colour entry has a colour to change")

// updateMediaItem changes the given fields of a library entry; nil leaves a
// field as is. hex changes a colour entry's colour.
func updateMediaItem(ctx context.Context, db *database.MongoDB, scope bson.M, id primitive.ObjectID, name, description, hex *string, tags []string, setTags bool) (models.MediaItem, error) {
	set := bson.M{"updatedAt": time.Now()}
	filter := bson.M{"_id": id}
	if hex != nil {
		norm, err := normalizeColor(*hex)
		if err != nil {
			return models.MediaItem{}, err
		}
		set["color"] = norm
		filter["contentType"] = models.MediaColorType
	}
	if name != nil && strings.TrimSpace(*name) != "" {
		set["name"] = strings.TrimSpace(*name)
	}
	if description != nil {
		set["description"] = strings.TrimSpace(*description)
	}
	if setTags {
		set["tags"] = normalizeTags(tags)
	}
	for k, v := range scope {
		filter[k] = v
	}
	var item models.MediaItem
	err := db.Media().FindOneAndUpdate(ctx, filter, bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&item)
	if err == mongo.ErrNoDocuments && hex != nil {
		delete(filter, "contentType")
		if n, _ := db.Media().CountDocuments(ctx, filter); n > 0 {
			return item, errNotAColor
		}
	}
	return item, err
}

func (h *MediaHandler) List(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	limit := 0
	fmt.Sscanf(c.Query("limit"), "%d", &limit)
	items, err := listMedia(ctx, h.DB, mcpScopeFilter(c), c.Query("q"), splitTags(c.Query("tags")), c.Query("kind"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load media"})
		return
	}
	c.JSON(http.StatusOK, items)
}

// Create takes a multipart upload: file (image, video, or a .ttf/.otf font),
// plus optional name, description and comma-separated tags.
func (h *MediaHandler) Create(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read the file"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 101*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read the file"})
		return
	}
	name := c.PostForm("name")
	if name == "" {
		name = strings.TrimSuffix(fh.Filename, filepath.Ext(fh.Filename))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	item, err := addMediaItem(ctx, c, h.DB, h.UploadDir, data, filepath.Ext(fh.Filename), name, c.PostForm("description"), splitTags(c.PostForm("tags")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

// CreateColor adds a colour: {color, name, description, tags}.
func (h *MediaHandler) CreateColor(c *gin.Context) {
	var input struct {
		Color       string   `json:"color" binding:"required"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	item, err := addMediaColor(ctx, c, h.DB, input.Color, input.Name, input.Description, input.Tags)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *MediaHandler) Update(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid media ID"})
		return
	}
	var input struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Color       *string   `json:"color"`
		Tags        *[]string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var tags []string
	if input.Tags != nil {
		tags = *input.Tags
	}
	item, err := updateMediaItem(ctx, h.DB, mcpScopeFilter(c), id, input.Name, input.Description, input.Color, tags, input.Tags != nil)
	if err == mongo.ErrNoDocuments {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *MediaHandler) Delete(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid media ID"})
		return
	}
	filter := mcpScopeFilter(c)
	filter["_id"] = id
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	urls, err := deleteOneAndCollectUploadURLs(ctx, h.DB.Media(), filter)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media not found"})
		return
	}
	cleanupUploads(h.DB, h.UploadDir, urls)
	c.JSON(http.StatusOK, gin.H{"message": "Media deleted"})
}

// maxFontSize caps an uploaded font; real-world TTF/OTF files of a single
// style stay well below it.
const maxFontSize = 10 << 20

// checkFontUpload accepts a TrueType (.ttf) or OpenType (.otf) font the
// image renderer can draw German text with, and returns the extension and content
// type to store it under. Those follow the file's contents rather than its
// name, since .otf files often hold TrueType outlines and vice versa.
func checkFontUpload(filename string, data []byte) (ext, contentType string, err error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".ttf", ".otf":
	default:
		return "", "", fmt.Errorf("upload a .ttf or .otf font file")
	}
	if len(data) > maxFontSize {
		return "", "", fmt.Errorf("the font file is larger than 10 MB")
	}
	if len(data) >= 4 && string(data[:4]) == "ttcf" {
		return "", "", fmt.Errorf("this is a font collection; upload a single .ttf or .otf font")
	}
	if _, err := services.ParseFont(data); err != nil {
		return "", "", fmt.Errorf("unusable font: %v", err)
	}
	if string(data[:4]) == "OTTO" {
		return ".otf", "font/otf", nil
	}
	return ".ttf", "font/ttf", nil
}
