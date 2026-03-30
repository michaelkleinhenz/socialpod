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

type SuffixHandler struct {
	DB *database.MongoDB
}

// suffixFilter returns a bson filter scoped to the user's team (if any) or user.
func suffixFilter(c *gin.Context) bson.M {
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		return bson.M{"teamId": tid}
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	return bson.M{"userId": uid}
}

func (h *SuffixHandler) List(c *gin.Context) {
	filter := suffixFilter(c)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := h.DB.Suffixes().Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch suffixes"})
		return
	}
	defer cursor.Close(ctx)

	var suffixes []models.Suffix
	if err := cursor.All(ctx, &suffixes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode suffixes"})
		return
	}
	if suffixes == nil {
		suffixes = []models.Suffix{}
	}
	c.JSON(http.StatusOK, suffixes)
}

func (h *SuffixHandler) Create(c *gin.Context) {
	var input struct {
		Name    string `json:"name" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	suffix := models.Suffix{
		UserID:    objID,
		Name:      input.Name,
		Content:   input.Content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		suffix.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Suffixes().InsertOne(ctx, suffix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create suffix"})
		return
	}

	suffix.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, suffix)
}

func (h *SuffixHandler) Update(c *gin.Context) {
	suffixID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suffix ID"})
		return
	}

	var input struct {
		Name    *string `json:"name"`
		Content *string `json:"content"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filter := suffixFilter(c)
	filter["_id"] = suffixID

	update := bson.M{"updatedAt": time.Now()}
	if input.Name != nil {
		update["name"] = *input.Name
	}
	if input.Content != nil {
		update["content"] = *input.Content
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Suffixes().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suffix not found"})
		return
	}

	var suffix models.Suffix
	h.DB.Suffixes().FindOne(ctx, bson.M{"_id": suffixID}).Decode(&suffix)
	c.JSON(http.StatusOK, suffix)
}

func (h *SuffixHandler) Delete(c *gin.Context) {
	suffixID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suffix ID"})
		return
	}

	filter := suffixFilter(c)
	filter["_id"] = suffixID

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Suffixes().DeleteOne(ctx, filter)
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suffix not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Suffix deleted"})
}
