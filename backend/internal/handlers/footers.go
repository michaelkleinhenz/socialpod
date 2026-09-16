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

type FooterHandler struct {
	DB *database.MongoDB
}

func footerFilter(c *gin.Context) bson.M {
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		return bson.M{"teamId": tid}
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	return bson.M{"userId": uid}
}

func (h *FooterHandler) List(c *gin.Context) {
	filter := footerFilter(c)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := h.DB.Footers().Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch footers"})
		return
	}
	defer cursor.Close(ctx)

	var footers []models.Footer
	if err := cursor.All(ctx, &footers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode footers"})
		return
	}
	if footers == nil {
		footers = []models.Footer{}
	}
	c.JSON(http.StatusOK, footers)
}

func (h *FooterHandler) Create(c *gin.Context) {
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

	footer := models.Footer{
		UserID:    objID,
		Name:      input.Name,
		Content:   input.Content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		footer.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Footers().InsertOne(ctx, footer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create footer"})
		return
	}

	footer.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, footer)
}

func (h *FooterHandler) Update(c *gin.Context) {
	footerID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid footer ID"})
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

	filter := footerFilter(c)
	filter["_id"] = footerID

	update := bson.M{"updatedAt": time.Now()}
	if input.Name != nil {
		update["name"] = *input.Name
	}
	if input.Content != nil {
		update["content"] = *input.Content
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Footers().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Footer not found"})
		return
	}

	var footer models.Footer
	h.DB.Footers().FindOne(ctx, bson.M{"_id": footerID}).Decode(&footer)
	c.JSON(http.StatusOK, footer)
}

func (h *FooterHandler) Delete(c *gin.Context) {
	footerID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid footer ID"})
		return
	}

	filter := footerFilter(c)
	filter["_id"] = footerID

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Footers().DeleteOne(ctx, filter)
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Footer not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Footer deleted"})
}
