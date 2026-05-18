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

type MentionHandler struct {
	DB *database.MongoDB
}

func mentionFilter(c *gin.Context) bson.M {
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		return bson.M{"teamId": tid}
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	return bson.M{"userId": uid}
}

func (h *MentionHandler) List(c *gin.Context) {
	filter := mentionFilter(c)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := h.DB.Mentions().Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch mentions"})
		return
	}
	defer cursor.Close(ctx)

	var mentions []models.MentionEntry
	if err := cursor.All(ctx, &mentions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode mentions"})
		return
	}
	if mentions == nil {
		mentions = []models.MentionEntry{}
	}
	c.JSON(http.StatusOK, mentions)
}

func (h *MentionHandler) Create(c *gin.Context) {
	var input struct {
		Name    string            `json:"name" binding:"required"`
		Handles map[string]string `json:"handles"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	if input.Handles == nil {
		input.Handles = map[string]string{}
	}

	mention := models.MentionEntry{
		UserID:    objID,
		Name:      input.Name,
		Handles:   input.Handles,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		mention.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Mentions().InsertOne(ctx, mention)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create mention"})
		return
	}

	mention.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, mention)
}

func (h *MentionHandler) Update(c *gin.Context) {
	mentionID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mention ID"})
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

	filter := mentionFilter(c)
	filter["_id"] = mentionID

	update := bson.M{"updatedAt": time.Now()}
	if input.Name != nil {
		update["name"] = *input.Name
	}
	if input.Handles != nil {
		update["handles"] = input.Handles
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Mentions().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Mention not found"})
		return
	}

	var mention models.MentionEntry
	h.DB.Mentions().FindOne(ctx, bson.M{"_id": mentionID}).Decode(&mention)
	c.JSON(http.StatusOK, mention)
}

func (h *MentionHandler) Delete(c *gin.Context) {
	mentionID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mention ID"})
		return
	}

	filter := mentionFilter(c)
	filter["_id"] = mentionID

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Mentions().DeleteOne(ctx, filter)
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Mention not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Mention deleted"})
}
