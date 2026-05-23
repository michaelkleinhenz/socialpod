package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ConventionHandler struct {
	DB        *database.MongoDB
	UploadDir string
}

func conventionQueueFilter(c *gin.Context) bson.M {
	if teamID, ok := c.Get("teamId"); ok && teamID.(string) != "" {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		return bson.M{"teamId": tid}
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	return bson.M{"userId": uid}
}

type ConventionQueueInput struct {
	Name          string            `json:"name" binding:"required"`
	ConventionURL string            `json:"conventionUrl,omitempty"`
	Hashtags      []string          `json:"hashtags,omitempty"`
	StartDate     string            `json:"startDate" binding:"required"`
	EndDate       string            `json:"endDate" binding:"required"`
	PostsPerDay   int               `json:"postsPerDay" binding:"required,min=1,max=5"`
	TimeSlots     []string          `json:"timeSlots" binding:"required"`
	Platforms     []models.Platform `json:"platforms" binding:"required"`
	AccountIDs    map[string]string `json:"accountIds,omitempty"`
	SuffixIDs     map[string]string `json:"suffixIds,omitempty"`
}

func (h *ConventionHandler) CreateQueue(c *gin.Context) {
	var input ConventionQueueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	startDate, err := time.Parse(time.RFC3339, input.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid startDate format, use RFC3339"})
		return
	}
	endDate, err := time.Parse(time.RFC3339, input.EndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid endDate format, use RFC3339"})
		return
	}
	if !endDate.After(startDate) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "endDate must be after startDate"})
		return
	}

	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))

	queue := models.ConventionQueue{
		UserID:        uid,
		Name:          input.Name,
		ConventionURL: input.ConventionURL,
		Hashtags:      input.Hashtags,
		StartDate:     startDate,
		EndDate:       endDate,
		PostsPerDay:   input.PostsPerDay,
		TimeSlots:     input.TimeSlots,
		Platforms:     input.Platforms,
		AccountIDs:    input.AccountIDs,
		SuffixIDs:     input.SuffixIDs,
		Status:        models.ConventionQueueStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if teamID, ok := c.Get("teamId"); ok && teamID.(string) != "" {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		queue.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.ConventionQueues().InsertOne(ctx, queue)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create queue"})
		return
	}

	queue.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, queue)
}

type conventionQueueWithCounts struct {
	models.ConventionQueue `bson:",inline"`
	ItemCount              int `bson:"-" json:"itemCount"`
	ApprovedCount          int `bson:"-" json:"approvedCount"`
	ScheduledCount         int `bson:"-" json:"scheduledCount"`
}

func (h *ConventionHandler) ListQueues(c *gin.Context) {
	filter := conventionQueueFilter(c)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	cursor, err := h.DB.ConventionQueues().Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch queues"})
		return
	}
	defer cursor.Close(ctx)

	var queues []models.ConventionQueue
	cursor.All(ctx, &queues)

	results := make([]conventionQueueWithCounts, 0, len(queues))
	for _, q := range queues {
		total, _ := h.DB.ConventionQueueItems().CountDocuments(ctx, bson.M{"queueId": q.ID})
		approved, _ := h.DB.ConventionQueueItems().CountDocuments(ctx, bson.M{"queueId": q.ID, "status": models.ConventionQueueItemStatusApproved})
		done, _ := h.DB.ConventionQueueItems().CountDocuments(ctx, bson.M{"queueId": q.ID, "status": bson.M{"$in": []string{models.ConventionQueueItemStatusScheduled, models.ConventionQueueItemStatusPublished}}})
		results = append(results, conventionQueueWithCounts{
			ConventionQueue: q,
			ItemCount:       int(total),
			ApprovedCount:   int(approved),
			ScheduledCount:  int(done),
		})
	}

	c.JSON(http.StatusOK, results)
}

func (h *ConventionHandler) GetQueue(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}

	filter := conventionQueueFilter(c)
	filter["_id"] = queueID

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var queue models.ConventionQueue
	if err := h.DB.ConventionQueues().FindOne(ctx, filter).Decode(&queue); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
		return
	}

	itemOpts := options.Find().SetSort(bson.D{{Key: "sortOrder", Value: 1}, {Key: "createdAt", Value: 1}})
	cursor, err := h.DB.ConventionQueueItems().Find(ctx, bson.M{"queueId": queueID}, itemOpts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch items"})
		return
	}
	defer cursor.Close(ctx)

	var items []models.ConventionQueueItem
	if err := cursor.All(ctx, &items); err != nil || items == nil {
		items = []models.ConventionQueueItem{}
	}

	c.JSON(http.StatusOK, gin.H{"queue": queue, "items": items})
}

func (h *ConventionHandler) UpdateQueue(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}

	var input ConventionQueueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	startDate, err := time.Parse(time.RFC3339, input.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid startDate format, use RFC3339"})
		return
	}
	endDate, err := time.Parse(time.RFC3339, input.EndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid endDate format, use RFC3339"})
		return
	}

	filter := conventionQueueFilter(c)
	filter["_id"] = queueID

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.ConventionQueues().UpdateOne(ctx, filter, bson.M{"$set": bson.M{
		"name":          input.Name,
		"conventionUrl": input.ConventionURL,
		"hashtags":      input.Hashtags,
		"startDate":     startDate,
		"endDate":       endDate,
		"postsPerDay":   input.PostsPerDay,
		"timeSlots":     input.TimeSlots,
		"platforms":     input.Platforms,
		"accountIds":    input.AccountIDs,
		"suffixIds":     input.SuffixIDs,
		"updatedAt":     time.Now(),
	}})
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
		return
	}

	var queue models.ConventionQueue
	h.DB.ConventionQueues().FindOne(ctx, bson.M{"_id": queueID}).Decode(&queue)
	c.JSON(http.StatusOK, queue)
}

func (h *ConventionHandler) DeleteQueue(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}

	filter := conventionQueueFilter(c)
	filter["_id"] = queueID

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.ConventionQueues().DeleteOne(ctx, filter)
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
		return
	}

	h.DB.ConventionQueueItems().DeleteMany(ctx, bson.M{"queueId": queueID})
	c.JSON(http.StatusOK, gin.H{"message": "Queue deleted"})
}

func (h *ConventionHandler) AddItem(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queueFilter := conventionQueueFilter(c)
	queueFilter["_id"] = queueID
	var queue models.ConventionQueue
	if err := h.DB.ConventionQueues().FindOne(ctx, queueFilter).Decode(&queue); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
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

	count, _ := h.DB.ConventionQueueItems().CountDocuments(ctx, bson.M{"queueId": queueID})

	item := models.ConventionQueueItem{
		QueueID:   queueID,
		ImageURL:  imageURL,
		Status:    models.ConventionQueueItemStatusPending,
		SortOrder: int(count),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	res, err := h.DB.ConventionQueueItems().InsertOne(ctx, item)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create item"})
		return
	}

	item.ID = res.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, item)
}

type UpdateItemInput struct {
	Caption    *string           `json:"caption,omitempty"`
	Status     *string           `json:"status,omitempty"`
	SortOrder  *int              `json:"sortOrder,omitempty"`
	BGGURL     *string           `json:"bggUrl,omitempty"`
	Platforms  []models.Platform `json:"platforms,omitempty"`
	AccountIDs map[string]string `json:"accountIds"`
	SuffixIDs  map[string]string `json:"suffixIds"`
}

func (h *ConventionHandler) UpdateItem(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}
	itemID, err := primitive.ObjectIDFromHex(c.Param("iid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	var input UpdateItemInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	update := bson.M{"updatedAt": time.Now()}
	if input.Caption != nil {
		update["caption"] = *input.Caption
	}
	if input.Status != nil {
		update["status"] = *input.Status
	}
	if input.SortOrder != nil {
		update["sortOrder"] = *input.SortOrder
	}
	if input.BGGURL != nil {
		update["bggUrl"] = *input.BGGURL
	}
	if input.Platforms != nil {
		update["platforms"] = input.Platforms
	}
	if input.AccountIDs != nil {
		update["accountIds"] = input.AccountIDs
	}
	if input.SuffixIDs != nil {
		update["suffixIds"] = input.SuffixIDs
	}

	result, err := h.DB.ConventionQueueItems().UpdateOne(ctx,
		bson.M{"_id": itemID, "queueId": queueID},
		bson.M{"$set": update},
	)
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found"})
		return
	}

	var item models.ConventionQueueItem
	h.DB.ConventionQueueItems().FindOne(ctx, bson.M{"_id": itemID}).Decode(&item)
	c.JSON(http.StatusOK, item)
}

func (h *ConventionHandler) ReplaceItemImage(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}
	itemID, err := primitive.ObjectIDFromHex(c.Param("iid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	result, err := h.DB.ConventionQueueItems().UpdateOne(ctx,
		bson.M{"_id": itemID, "queueId": queueID},
		bson.M{"$set": bson.M{"imageUrl": imageURL, "updatedAt": time.Now()}},
	)
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found"})
		return
	}

	var item models.ConventionQueueItem
	h.DB.ConventionQueueItems().FindOne(ctx, bson.M{"_id": itemID}).Decode(&item)
	c.JSON(http.StatusOK, item)
}

func (h *ConventionHandler) DeleteItem(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}
	itemID, err := primitive.ObjectIDFromHex(c.Param("iid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.DB.ConventionQueueItems().DeleteOne(ctx, bson.M{"_id": itemID, "queueId": queueID})
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Item deleted"})
}

func (h *ConventionHandler) AnalyzeItem(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}
	itemID, err := primitive.ObjectIDFromHex(c.Param("iid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	queueFilter := conventionQueueFilter(c)
	queueFilter["_id"] = queueID
	var queue models.ConventionQueue
	if err := h.DB.ConventionQueues().FindOne(ctx, queueFilter).Decode(&queue); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
		return
	}

	var item models.ConventionQueueItem
	if err := h.DB.ConventionQueueItems().FindOne(ctx, bson.M{"_id": itemID, "queueId": queueID}).Decode(&item); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found"})
		return
	}

	bggInfo := h.fetchBGGGameInfo(ctx, item.BGGURL)
	caption, aiErr := h.generateCaption(ctx, item.ImageURL, queue, bggInfo)
	now := time.Now()
	if aiErr != nil {
		h.DB.ConventionQueueItems().UpdateOne(ctx,
			bson.M{"_id": itemID},
			bson.M{"$set": bson.M{"aiError": aiErr.Error(), "updatedAt": now}},
		)
		c.JSON(http.StatusBadGateway, gin.H{"error": aiErr.Error()})
		return
	}

	h.DB.ConventionQueueItems().UpdateOne(ctx,
		bson.M{"_id": itemID},
		bson.M{"$set": bson.M{"caption": caption, "aiError": "", "updatedAt": now}},
	)

	h.DB.ConventionQueueItems().FindOne(ctx, bson.M{"_id": itemID}).Decode(&item)
	c.JSON(http.StatusOK, item)
}

func (h *ConventionHandler) AnalyzeAll(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queueFilter := conventionQueueFilter(c)
	queueFilter["_id"] = queueID
	var queue models.ConventionQueue
	if err := h.DB.ConventionQueues().FindOne(ctx, queueFilter).Decode(&queue); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
		return
	}

	cursor, err := h.DB.ConventionQueueItems().Find(ctx,
		bson.M{"queueId": queueID, "status": models.ConventionQueueItemStatusPending},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch items"})
		return
	}
	var items []models.ConventionQueueItem
	cursor.All(ctx, &items)
	cursor.Close(ctx)

	if len(items) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "No pending items to analyze", "count": 0})
		return
	}

	go func(items []models.ConventionQueueItem, queue models.ConventionQueue) {
		sem := make(chan struct{}, 3)
		var wg sync.WaitGroup
		for _, item := range items {
			wg.Add(1)
			sem <- struct{}{}
			go func(it models.ConventionQueueItem) {
				defer wg.Done()
				defer func() { <-sem }()
				bgCtx, bgCancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer bgCancel()
				bggInfo := h.fetchBGGGameInfo(bgCtx, it.BGGURL)
				caption, aiErr := h.generateCaption(bgCtx, it.ImageURL, queue, bggInfo)
				now := time.Now()
				if aiErr != nil {
					h.DB.ConventionQueueItems().UpdateOne(bgCtx,
						bson.M{"_id": it.ID},
						bson.M{"$set": bson.M{"aiError": aiErr.Error(), "updatedAt": now}},
					)
					return
				}
				h.DB.ConventionQueueItems().UpdateOne(bgCtx,
					bson.M{"_id": it.ID},
					bson.M{"$set": bson.M{"caption": caption, "aiError": "", "updatedAt": now}},
				)
			}(item)
		}
		wg.Wait()
	}(items, queue)

	c.JSON(http.StatusAccepted, gin.H{
		"message": fmt.Sprintf("Analyzing %d items in the background", len(items)),
		"count":   len(items),
	})
}

func (h *ConventionHandler) ReorderItems(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}

	var input struct {
		ItemIDs []string `json:"itemIds" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i, idStr := range input.ItemIDs {
		itemID, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			continue
		}
		h.DB.ConventionQueueItems().UpdateOne(ctx,
			bson.M{"_id": itemID, "queueId": queueID},
			bson.M{"$set": bson.M{"sortOrder": i, "updatedAt": time.Now()}},
		)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Reordered"})
}

type scheduleSlot struct {
	ItemID      string    `json:"itemId"`
	ScheduledAt time.Time `json:"scheduledAt"`
}

func (h *ConventionHandler) generateSlots(startDate, endDate time.Time, postsPerDay int, timeSlots []string) []time.Time {
	var slots []time.Time
	now := time.Now()
	loc := startDate.Location()
	current := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, loc)
	end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, loc)

	for !current.After(end) {
		for i := 0; i < postsPerDay && i < len(timeSlots); i++ {
			parts := strings.SplitN(timeSlots[i], ":", 2)
			if len(parts) != 2 {
				continue
			}
			hh, _ := strconv.Atoi(parts[0])
			mm, _ := strconv.Atoi(parts[1])
			slot := time.Date(current.Year(), current.Month(), current.Day(), hh, mm, 0, 0, loc)
			if slot.After(now) {
				slots = append(slots, slot)
			}
		}
		current = current.AddDate(0, 0, 1)
	}
	return slots
}

func (h *ConventionHandler) PreviewSchedule(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queueFilter := conventionQueueFilter(c)
	queueFilter["_id"] = queueID
	var queue models.ConventionQueue
	if err := h.DB.ConventionQueues().FindOne(ctx, queueFilter).Decode(&queue); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
		return
	}

	itemOpts := options.Find().SetSort(bson.D{{Key: "sortOrder", Value: 1}, {Key: "createdAt", Value: 1}})
	cursor, err := h.DB.ConventionQueueItems().Find(ctx,
		bson.M{"queueId": queueID, "status": models.ConventionQueueItemStatusApproved},
		itemOpts,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch items"})
		return
	}
	var items []models.ConventionQueueItem
	cursor.All(ctx, &items)
	cursor.Close(ctx)

	slots := h.generateSlots(queue.StartDate, queue.EndDate, queue.PostsPerDay, queue.TimeSlots)

	preview := make([]scheduleSlot, 0, len(items))
	for i, item := range items {
		if i >= len(slots) {
			break
		}
		preview = append(preview, scheduleSlot{
			ItemID:      item.ID.Hex(),
			ScheduledAt: slots[i],
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"slots":          preview,
		"approvedCount":  len(items),
		"availableSlots": len(slots),
		"overflow":       max(0, len(items)-len(slots)),
	})
}

func (h *ConventionHandler) ScheduleItems(c *gin.Context) {
	queueID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queueFilter := conventionQueueFilter(c)
	queueFilter["_id"] = queueID
	var queue models.ConventionQueue
	if err := h.DB.ConventionQueues().FindOne(ctx, queueFilter).Decode(&queue); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue not found"})
		return
	}

	itemOpts := options.Find().SetSort(bson.D{{Key: "sortOrder", Value: 1}, {Key: "createdAt", Value: 1}})
	cursor, err := h.DB.ConventionQueueItems().Find(ctx,
		bson.M{"queueId": queueID, "status": models.ConventionQueueItemStatusApproved},
		itemOpts,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch items"})
		return
	}
	var items []models.ConventionQueueItem
	cursor.All(ctx, &items)
	cursor.Close(ctx)

	if len(items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No approved items to schedule"})
		return
	}

	slots := h.generateSlots(queue.StartDate, queue.EndDate, queue.PostsPerDay, queue.TimeSlots)
	if len(slots) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No available time slots in the specified window — extend the date range or increase posts per day"})
		return
	}

	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))

	var teamID *primitive.ObjectID
	if tid, ok := c.Get("teamId"); ok && tid.(string) != "" {
		t, _ := primitive.ObjectIDFromHex(tid.(string))
		teamID = &t
	}

	scheduled := 0
	for i, item := range items {
		if i >= len(slots) {
			break
		}

		platforms := item.Platforms
		if len(platforms) == 0 {
			platforms = queue.Platforms
		}
		accountIDs := item.AccountIDs
		if len(accountIDs) == 0 {
			accountIDs = queue.AccountIDs
		}

		post := models.Post{
			UserID:      uid,
			TeamID:      teamID,
			PostType:    models.PostTypePost,
			Content:     item.Caption,
			ImageURLs:   []string{item.ImageURL},
			Platforms:   platforms,
			ScheduledAt: slots[i],
			Status:      models.PostStatusScheduled,
			AccountIDs:  accountIDs,
			SuffixIDs:   func() map[string]string {
				if len(item.SuffixIDs) > 0 {
					return item.SuffixIDs
				}
				return queue.SuffixIDs
			}(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		res, err := h.DB.Posts().InsertOne(ctx, post)
		if err != nil {
			continue
		}

		postID := res.InsertedID.(primitive.ObjectID)
		h.DB.ConventionQueueItems().UpdateOne(ctx,
			bson.M{"_id": item.ID},
			bson.M{"$set": bson.M{
				"status":    models.ConventionQueueItemStatusScheduled,
				"postId":    postID,
				"updatedAt": time.Now(),
			}},
		)
		scheduled++
	}

	c.JSON(http.StatusOK, gin.H{
		"scheduled": scheduled,
		"overflow":  max(0, len(items)-len(slots)),
	})
}

type bggGameInfo struct {
	Title       string
	Designers   []string
	Description string
}

func (h *ConventionHandler) fetchBGGGameInfo(ctx context.Context, bggURL string) *bggGameInfo {
	if bggURL == "" {
		return nil
	}
	m := bggGameIDRe.FindStringSubmatch(bggURL)
	if m == nil {
		return nil
	}
	gameID := m[1]

	var settings models.AppSettings
	h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)

	item, err := fetchBGGItem(ctx, gameID, settings.BGGAPIToken)
	if err != nil {
		return nil
	}

	title := ""
	for _, n := range item.Names {
		if n.Type == "primary" {
			title = n.Value
			break
		}
	}
	if title == "" && len(item.Names) > 0 {
		title = item.Names[0].Value
	}

	var designers []string
	for _, link := range item.Links {
		if link.Type == "boardgamedesigner" {
			designers = append(designers, link.Value)
		}
	}

	return &bggGameInfo{
		Title:       title,
		Designers:   designers,
		Description: cleanBGGText(item.Desc),
	}
}

func (h *ConventionHandler) generateCaption(ctx context.Context, imageURL string, queue models.ConventionQueue, bggInfo *bggGameInfo) (string, error) {
	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil || settings.OpenRouterAPIKey == "" {
		return "", fmt.Errorf("OpenRouter is not configured")
	}

	model := settings.OpenRouterModel
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	imageData, contentType, err := h.readImageData(ctx, imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to read image: %w", err)
	}

	var contextLines []string
	contextLines = append(contextLines, fmt.Sprintf("Convention: %s", queue.Name))
	if queue.ConventionURL != "" {
		contextLines = append(contextLines, fmt.Sprintf("Convention URL: %s", queue.ConventionURL))
	}
	if bggInfo != nil {
		contextLines = append(contextLines, fmt.Sprintf("Board game shown: %s", bggInfo.Title))
		if len(bggInfo.Designers) > 0 {
			contextLines = append(contextLines, fmt.Sprintf("Designers: %s", strings.Join(bggInfo.Designers, ", ")))
		}
		if bggInfo.Description != "" {
			desc := bggInfo.Description
			if len(desc) > 300 {
				desc = desc[:300] + "…"
			}
			contextLines = append(contextLines, fmt.Sprintf("Game description: %s", desc))
		}
	}
	if len(queue.Hashtags) > 0 {
		contextLines = append(contextLines, fmt.Sprintf("Hashtags to include: %s", strings.Join(queue.Hashtags, " ")))
	}
	if len(queue.Platforms) > 0 {
		limit := contentLimit(queue.Platforms)
		contextLines = append(contextLines, fmt.Sprintf("Character limit: %d characters maximum (including hashtags).", limit))
	}
	contextLines = append(contextLines, "\nWrite an engaging social media caption for this convention photo. Reply with ONLY the caption text including the hashtags.")

	imgB64 := base64.StdEncoding.EncodeToString(imageData)
	userContent := []map[string]any{
		{
			"type": "image_url",
			"image_url": map[string]string{
				"url": fmt.Sprintf("data:%s;base64,%s", contentType, imgB64),
			},
		},
		{"type": "text", "text": strings.Join(contextLines, "\n")},
	}

	systemPrompt := "You are a board game convention social media expert. Look at photos from game conventions (Essen, Gen Con, SPIEL, etc.) and write engaging ready-to-post social media captions. Describe what you see — publisher booths, games on tables, prototypes, crowds — with enthusiasm. Reply with ONLY the caption text, no quotes, no commentary."
	if settings.AILanguage != "" {
		systemPrompt += fmt.Sprintf(" Write in %s.", settings.AILanguage)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+settings.OpenRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to reach OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenRouter returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) == 0 {
		return "", fmt.Errorf("invalid response from OpenRouter")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func (h *ConventionHandler) readImageData(ctx context.Context, imageURL string) ([]byte, string, error) {
	filename := filepath.Base(imageURL)
	diskPath := filepath.Join(h.UploadDir, filename)

	data, err := os.ReadFile(diskPath)
	if err == nil {
		ct := http.DetectContentType(data)
		return data, ct, nil
	}

	var upload models.Upload
	if err := h.DB.Uploads().FindOne(ctx, bson.M{"filename": filename}).Decode(&upload); err != nil {
		return nil, "", fmt.Errorf("image not found: %s", filename)
	}
	if len(upload.Data) == 0 {
		return nil, "", fmt.Errorf("image data is empty for: %s", filename)
	}
	ct := upload.ContentType
	if ct == "" {
		ct = http.DetectContentType(upload.Data)
	}
	return upload.Data, ct, nil
}
