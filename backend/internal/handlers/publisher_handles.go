package handlers

import (
	"context"
	"net/http"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PublisherHandleHandler struct {
	DB *database.MongoDB
}

func (h *PublisherHandleHandler) List(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := h.DB.PublisherHandles().Find(ctx, bson.M{}, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch publisher handles"})
		return
	}
	defer cursor.Close(ctx)

	var items []models.PublisherHandle
	if err := cursor.All(ctx, &items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode publisher handles"})
		return
	}
	if items == nil {
		items = []models.PublisherHandle{}
	}
	c.JSON(http.StatusOK, items)
}

func (h *PublisherHandleHandler) Create(c *gin.Context) {
	var input struct {
		Name    string            `json:"name" binding:"required"`
		Handles map[string]string `json:"handles"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Handles == nil {
		input.Handles = map[string]string{}
	}

	entry := models.PublisherHandle{
		Name:      input.Name,
		Handles:   input.Handles,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.PublisherHandles().InsertOne(ctx, entry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create publisher handle"})
		return
	}
	entry.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, entry)
}

func (h *PublisherHandleHandler) Update(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var input struct {
		Name    *string           `json:"name"`
		Handles map[string]string `json:"handles"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	update := bson.M{"updatedAt": time.Now()}
	if input.Name != nil {
		update["name"] = *input.Name
	}
	if input.Handles != nil {
		update["handles"] = input.Handles
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.PublisherHandles().UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Publisher handle not found"})
		return
	}

	var entry models.PublisherHandle
	h.DB.PublisherHandles().FindOne(ctx, bson.M{"_id": id}).Decode(&entry)
	c.JSON(http.StatusOK, entry)
}

func (h *PublisherHandleHandler) Delete(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.PublisherHandles().DeleteOne(ctx, bson.M{"_id": id})
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Publisher handle not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Deleted"})
}
