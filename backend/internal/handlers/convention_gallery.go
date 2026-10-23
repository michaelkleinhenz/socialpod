package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// A queue's gallery holds photos uploaded without deciding yet what they
// become. The queue page shows them all, and the user moves photos out of it
// into a single-post item each (pending, as if uploaded directly) or into a
// carousel item, a new one or one the queue already has.

const (
	galleryMoveSingle   = "single"
	galleryMoveCarousel = "carousel"
)

// listGallery returns a queue's gallery photos, oldest first. A failed read
// yields an empty list, so the queue page still loads.
func (h *ConventionHandler) listGallery(ctx context.Context, queueID primitive.ObjectID) []models.ConventionGalleryPhoto {
	photos := []models.ConventionGalleryPhoto{}
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}})
	cursor, err := h.DB.ConventionGallery().Find(ctx, bson.M{"queueId": queueID}, opts)
	if err != nil {
		return photos
	}
	defer cursor.Close(ctx)
	if err := cursor.All(ctx, &photos); err != nil || photos == nil {
		return []models.ConventionGalleryPhoto{}
	}
	return photos
}

// findOwnQueue loads the queue named by the :id parameter if it belongs to the
// caller (their team, or them), answering the request itself otherwise.
func (h *ConventionHandler) findOwnQueue(c *gin.Context, ctx context.Context) (models.ConventionQueue, bool) {
	var queue models.ConventionQueue
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return queue, false
	}
	filter := conventionQueueFilter(c)
	filter["_id"] = queueID
	if err := h.DB.ConventionQueues().FindOne(ctx, filter).Decode(&queue); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
		return queue, false
	}
	return queue, true
}

// AddGalleryPhotos stores every uploaded file (multipart "images") as a photo
// of the queue's gallery.
func (h *ConventionHandler) AddGalleryPhotos(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	queue, ok := h.findOwnQueue(c, ctx)
	if !ok {
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data"})
		return
	}
	files := form.File["images"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image files provided"})
		return
	}

	ph := &PostHandler{DB: h.DB, UploadDir: h.UploadDir}
	photos := make([]models.ConventionGalleryPhoto, 0, len(files))
	docs := make([]interface{}, 0, len(files))
	now := time.Now()
	for i, fh := range files {
		url, err := ph.saveUpload(ctx, fh)
		if err != nil {
			urls := make([]string, 0, len(photos))
			for _, p := range photos {
				urls = append(urls, p.ImageURL)
			}
			cleanupUploads(h.DB, h.UploadDir, urls)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed: " + err.Error()})
			return
		}
		// Keep the picked order: each photo a millisecond after the previous.
		photo := models.ConventionGalleryPhoto{
			ID:        primitive.NewObjectID(),
			QueueID:   queue.ID,
			ImageURL:  url,
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
		photos = append(photos, photo)
		docs = append(docs, photo)
	}

	if _, err := h.DB.ConventionGallery().InsertMany(ctx, docs); err != nil {
		urls := make([]string, 0, len(photos))
		for _, p := range photos {
			urls = append(urls, p.ImageURL)
		}
		cleanupUploads(h.DB, h.UploadDir, urls)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store photos"})
		return
	}
	c.JSON(http.StatusCreated, photos)
}

// DeleteGalleryPhoto removes a photo from the gallery and frees its file.
func (h *ConventionHandler) DeleteGalleryPhoto(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queue, ok := h.findOwnQueue(c, ctx)
	if !ok {
		return
	}
	photoID, err := primitive.ObjectIDFromHex(c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid photo ID"})
		return
	}

	var photo models.ConventionGalleryPhoto
	filter := bson.M{"_id": photoID, "queueId": queue.ID}
	if err := h.DB.ConventionGallery().FindOneAndDelete(ctx, filter).Decode(&photo); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Photo not found"})
		return
	}
	cleanupUploads(h.DB, h.UploadDir, []string{photo.ImageURL})
	c.JSON(http.StatusOK, gin.H{"message": "Photo deleted"})
}

// ReplaceGalleryPhoto swaps a gallery photo's image for the uploaded one
// (multipart "image"), a crop of it, and frees the old file.
func (h *ConventionHandler) ReplaceGalleryPhoto(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queue, ok := h.findOwnQueue(c, ctx)
	if !ok {
		return
	}
	photoID, err := primitive.ObjectIDFromHex(c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid photo ID"})
		return
	}
	_, fh, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file provided"})
		return
	}

	ph := &PostHandler{DB: h.DB, UploadDir: h.UploadDir}
	imageURL, err := ph.saveUpload(ctx, fh)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed: " + err.Error()})
		return
	}

	// The photo may have been moved out or deleted meanwhile; then the new
	// file belongs to nothing.
	var previous models.ConventionGalleryPhoto
	filter := bson.M{"_id": photoID, "queueId": queue.ID}
	if err := h.DB.ConventionGallery().FindOneAndUpdate(ctx, filter,
		bson.M{"$set": bson.M{"imageUrl": imageURL}},
	).Decode(&previous); err != nil {
		cleanupUploads(h.DB, h.UploadDir, []string{imageURL})
		c.JSON(http.StatusNotFound, gin.H{"error": "Photo not found"})
		return
	}
	cleanupUploads(h.DB, h.UploadDir, []string{previous.ImageURL})

	photo := previous
	photo.ImageURL = imageURL
	c.JSON(http.StatusOK, photo)
}

type MoveGalleryPhotosInput struct {
	PhotoIDs []string `json:"photoIds" binding:"required,min=1"`
	// Target is "single" (one pending single-post item per photo) or
	// "carousel" (the photos go into one carousel item).
	Target string `json:"target" binding:"required"`
	// ItemID names the carousel item to add the photos to; empty creates a
	// new carousel item. Only used with target "carousel".
	ItemID string `json:"itemId,omitempty"`
}

// MoveGalleryPhotos moves photos out of the gallery into the queue: each into
// a single-post item of its own, or all of them, in the order given, into a
// carousel item. The items are pending, like freshly uploaded ones. The image
// files pass to the items unchanged, so nothing is freed.
func (h *ConventionHandler) MoveGalleryPhotos(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	queue, ok := h.findOwnQueue(c, ctx)
	if !ok {
		return
	}

	var input MoveGalleryPhotosInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Target != galleryMoveSingle && input.Target != galleryMoveCarousel {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target must be \"single\" or \"carousel\""})
		return
	}

	ids := make([]primitive.ObjectID, 0, len(input.PhotoIDs))
	seen := map[primitive.ObjectID]bool{}
	for _, s := range input.PhotoIDs {
		id, err := primitive.ObjectIDFromHex(s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid photo ID"})
			return
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	// The carousel the photos join, checked before anything moves.
	var target *models.ConventionQueueItem
	if input.Target == galleryMoveCarousel && input.ItemID != "" {
		itemID, err := primitive.ObjectIDFromHex(input.ItemID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
			return
		}
		var item models.ConventionQueueItem
		if err := h.DB.ConventionQueueItems().FindOne(ctx, bson.M{"_id": itemID, "queueId": queue.ID}).Decode(&item); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Carousel not found"})
			return
		}
		if item.Status == models.ConventionQueueItemStatusScheduled {
			c.JSON(http.StatusConflict, gin.H{"error": "That carousel is already scheduled"})
			return
		}
		target = &item
	}

	cursor, err := h.DB.ConventionGallery().Find(ctx, bson.M{"_id": bson.M{"$in": ids}, "queueId": queue.ID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch photos"})
		return
	}
	var found []models.ConventionGalleryPhoto
	cursor.All(ctx, &found)
	cursor.Close(ctx)
	photos, ok := orderGalleryPhotos(found, ids)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Some photos are no longer in the gallery"})
		return
	}

	// Take the photos out first, one by one: a second move of the same photos,
	// racing this one, then finds them gone instead of placing them twice.
	taken := make([]models.ConventionGalleryPhoto, 0, len(photos))
	for _, p := range photos {
		del, err := h.DB.ConventionGallery().DeleteOne(ctx, bson.M{"_id": p.ID, "queueId": queue.ID})
		if err != nil || del.DeletedCount == 0 {
			// Put back only what this call removed before giving up.
			h.restoreGalleryPhotos(ctx, taken)
			c.JSON(http.StatusConflict, gin.H{"error": "Some photos are no longer in the gallery"})
			return
		}
		taken = append(taken, p)
	}

	urls := make([]string, len(photos))
	for i, p := range photos {
		urls[i] = p.ImageURL
	}

	items, err := h.placeGalleryPhotos(ctx, queue.ID, input.Target, target, urls)
	if err != nil {
		h.restoreGalleryPhotos(ctx, photos)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// placeGalleryPhotos creates or updates the queue items the moved images go
// into and returns them.
func (h *ConventionHandler) placeGalleryPhotos(ctx context.Context, queueID primitive.ObjectID, mode string, target *models.ConventionQueueItem, urls []string) ([]models.ConventionQueueItem, error) {
	count, _ := h.DB.ConventionQueueItems().CountDocuments(ctx, bson.M{"queueId": queueID})
	now := time.Now()
	newItem := func(imageURLs []string, sortOrder int) models.ConventionQueueItem {
		return models.ConventionQueueItem{
			ID:          primitive.NewObjectID(),
			QueueID:     queueID,
			ImageURL:    imageURLs[0],
			ImageURLs:   imageURLs,
			Status:      models.ConventionQueueItemStatusPending,
			FromGallery: true,
			SortOrder:   sortOrder,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	switch {
	case mode == galleryMoveSingle:
		items := make([]models.ConventionQueueItem, len(urls))
		docs := make([]interface{}, len(urls))
		for i, url := range urls {
			// A single-post item has only ImageURL, like a direct upload.
			item := newItem([]string{url}, int(count)+i)
			item.ImageURLs = nil
			item.CreatedAt = now.Add(time.Duration(i) * time.Millisecond)
			item.UpdatedAt = item.CreatedAt
			items[i], docs[i] = item, item
		}
		if _, err := h.DB.ConventionQueueItems().InsertMany(ctx, docs); err != nil {
			return nil, errGalleryMove
		}
		return items, nil

	case target == nil:
		item := newItem(urls, int(count))
		if _, err := h.DB.ConventionQueueItems().InsertOne(ctx, item); err != nil {
			return nil, errGalleryMove
		}
		return []models.ConventionQueueItem{item}, nil

	default:
		set := appendCarouselImages(*target, urls)
		set["fromGallery"] = true
		set["updatedAt"] = now
		res, err := h.DB.ConventionQueueItems().UpdateOne(ctx,
			bson.M{"_id": target.ID, "status": bson.M{"$ne": models.ConventionQueueItemStatusScheduled}},
			bson.M{"$set": set},
		)
		if err != nil || res.MatchedCount == 0 {
			return nil, errGalleryMove
		}
		var item models.ConventionQueueItem
		if err := h.DB.ConventionQueueItems().FindOne(ctx, bson.M{"_id": target.ID}).Decode(&item); err != nil {
			return nil, errGalleryMove
		}
		return []models.ConventionQueueItem{item}, nil
	}
}

var errGalleryMove = errors.New("Failed to move the photos into the queue")

// restoreGalleryPhotos puts photos back into the gallery after a failed move.
// Photos still there are rewritten unchanged.
func (h *ConventionHandler) restoreGalleryPhotos(ctx context.Context, photos []models.ConventionGalleryPhoto) {
	for _, p := range photos {
		h.DB.ConventionGallery().ReplaceOne(ctx, bson.M{"_id": p.ID}, p, options.Replace().SetUpsert(true))
	}
}

// returnToGallery puts the images of a deleted item back into the queue's
// gallery, each once, in the item's order, and returns the new photos.
func (h *ConventionHandler) returnToGallery(ctx context.Context, queueID primitive.ObjectID, urls []string) ([]models.ConventionGalleryPhoto, error) {
	urls = uniqueURLs(urls)
	photos := make([]models.ConventionGalleryPhoto, 0, len(urls))
	if len(urls) == 0 {
		return photos, nil
	}
	docs := make([]interface{}, 0, len(urls))
	now := time.Now()
	for i, url := range urls {
		photo := models.ConventionGalleryPhoto{
			ID:        primitive.NewObjectID(),
			QueueID:   queueID,
			ImageURL:  url,
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
		photos = append(photos, photo)
		docs = append(docs, photo)
	}
	if _, err := h.DB.ConventionGallery().InsertMany(ctx, docs); err != nil {
		return nil, err
	}
	return photos, nil
}

// uniqueURLs returns the non-empty urls, each once, in their first order.
func uniqueURLs(urls []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// orderGalleryPhotos returns the photos in the order of ids, and false when one
// of the ids has no photo.
func orderGalleryPhotos(photos []models.ConventionGalleryPhoto, ids []primitive.ObjectID) ([]models.ConventionGalleryPhoto, bool) {
	byID := make(map[primitive.ObjectID]models.ConventionGalleryPhoto, len(photos))
	for _, p := range photos {
		byID[p.ID] = p
	}
	ordered := make([]models.ConventionGalleryPhoto, 0, len(ids))
	for _, id := range ids {
		p, ok := byID[id]
		if !ok {
			return nil, false
		}
		ordered = append(ordered, p)
	}
	return ordered, true
}

// appendCarouselImages works out the update that adds urls to the end of an
// item's images. A plain item, which has only ImageURL, becomes a carousel
// that starts with it; ImageURL keeps mirroring the first image.
func appendCarouselImages(item models.ConventionQueueItem, urls []string) bson.M {
	images := append([]string(nil), item.ImageURLs...)
	if len(images) == 0 && item.ImageURL != "" {
		images = append(images, item.ImageURL)
	}
	images = append(images, urls...)
	return bson.M{"imageUrls": images, "imageUrl": images[0]}
}
