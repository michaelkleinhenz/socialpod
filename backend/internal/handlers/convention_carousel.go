package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// A carousel item's pages can be added to (an uploaded photo, or an image made
// in the image editor, e.g. a front or back page), put in another order and
// taken out, until the item is scheduled.

// findOpenItem loads the :iid item of a queue for a change to its images,
// answering the request itself when it is missing or already scheduled.
func (h *ConventionHandler) findOpenItem(c *gin.Context, ctx context.Context, queueID primitive.ObjectID) (models.ConventionQueueItem, bool) {
	var item models.ConventionQueueItem
	itemID, err := primitive.ObjectIDFromHex(c.Param("iid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return item, false
	}
	if err := h.DB.ConventionQueueItems().FindOne(ctx, bson.M{"_id": itemID, "queueId": queueID}).Decode(&item); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found"})
		return item, false
	}
	// A scheduled item's post already carries its images.
	if item.Status == models.ConventionQueueItemStatusScheduled {
		c.JSON(http.StatusConflict, gin.H{"error": "Item is already scheduled"})
		return item, false
	}
	return item, true
}

// itemImages returns an item's images in order: a carousel's list, or a plain
// item's only image.
func itemImages(item models.ConventionQueueItem) []string {
	if len(item.ImageURLs) > 0 {
		return item.ImageURLs
	}
	if item.ImageURL != "" {
		return []string{item.ImageURL}
	}
	return nil
}

// saveItemImages stores images as the item's pages, guarded on the item not
// having been scheduled meanwhile, and answers with the updated item.
func (h *ConventionHandler) saveItemImages(c *gin.Context, ctx context.Context, item models.ConventionQueueItem, images []string) bool {
	res, err := h.DB.ConventionQueueItems().UpdateOne(ctx,
		bson.M{"_id": item.ID, "status": bson.M{"$ne": models.ConventionQueueItemStatusScheduled}},
		bson.M{"$set": bson.M{"imageUrls": images, "imageUrl": images[0], "updatedAt": time.Now()}},
	)
	if err != nil || res.MatchedCount == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Item is already scheduled"})
		return false
	}
	h.DB.ConventionQueueItems().FindOne(ctx, bson.M{"_id": item.ID}).Decode(&item)
	c.JSON(http.StatusOK, item)
	return true
}

// AddItemImage adds an image (multipart "image") to an item as a carousel
// page, at "index" (0 = the new front page) or, without one, at the end. A
// plain item becomes a carousel that keeps its image.
func (h *ConventionHandler) AddItemImage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queue, ok := h.findOwnQueue(c, ctx)
	if !ok {
		return
	}
	item, ok := h.findOpenItem(c, ctx, queue.ID)
	if !ok {
		return
	}
	_, fh, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file provided"})
		return
	}
	images := itemImages(item)
	index := len(images)
	if s := c.Request.FormValue("index"); s != "" {
		if index, err = strconv.Atoi(s); err != nil || index < 0 || index > len(images) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image index"})
			return
		}
	}

	ph := &PostHandler{DB: h.DB, UploadDir: h.UploadDir}
	imageURL, err := ph.saveUpload(ctx, fh)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed: " + err.Error()})
		return
	}
	if !h.saveItemImages(c, ctx, item, insertImage(images, index, imageURL)) {
		cleanupUploads(h.DB, h.UploadDir, []string{imageURL})
	}
}

type SetItemImagesInput struct {
	ImageURLs []string `json:"imageUrls" binding:"required,min=1"`
}

// SetItemImages puts an item's images in the order given and drops those left
// out, freeing their files. It takes only images the item has.
func (h *ConventionHandler) SetItemImages(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queue, ok := h.findOwnQueue(c, ctx)
	if !ok {
		return
	}
	item, ok := h.findOpenItem(c, ctx, queue.ID)
	if !ok {
		return
	}
	var input SetItemImagesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	current := itemImages(item)
	dropped, ok := droppedImages(current, input.ImageURLs)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "imageUrls must be images of this item, each at most once"})
		return
	}
	if h.saveItemImages(c, ctx, item, input.ImageURLs) {
		cleanupUploads(h.DB, h.UploadDir, dropped)
	}
}

// insertImage returns images with url inserted at index.
func insertImage(images []string, index int, url string) []string {
	out := make([]string, 0, len(images)+1)
	out = append(out, images[:index]...)
	out = append(out, url)
	return append(out, images[index:]...)
}

// droppedImages checks that next only holds images of current, each at most
// as often as current has it, and returns the ones next leaves out.
func droppedImages(current, next []string) ([]string, bool) {
	left := map[string]int{}
	for _, u := range current {
		left[u]++
	}
	for _, u := range next {
		if left[u] == 0 {
			return nil, false
		}
		left[u]--
	}
	var dropped []string
	for _, u := range current {
		if left[u] > 0 {
			dropped = append(dropped, u)
			left[u]--
		}
	}
	return dropped, true
}
