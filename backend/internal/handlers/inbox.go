package handlers

import (
	"context"
	"net/http"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"
	"socialmedia/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type InboxHandler struct {
	DB        *database.MongoDB
	Instagram *services.InstagramService
}

// ListComments returns paginated comment inbox entries across all platforms.
// GET /inbox/comments?page=1&limit=50
func (h *InboxHandler) ListComments(c *gin.Context) {
	h.listMessages(c, models.MessageTypeComment)
}

// ListDMs returns paginated DM inbox entries across all platforms.
// GET /inbox/dms?page=1&limit=50
func (h *InboxHandler) ListDMs(c *gin.Context) {
	h.listMessages(c, models.MessageTypeDM)
}

func (h *InboxHandler) listMessages(c *gin.Context, msgType models.MessageType) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"messageType": msgType}

	opts := options.Find().
		SetSort(bson.D{{Key: "receivedAt", Value: -1}}).
		SetLimit(100)

	cursor, err := h.DB.InboxMessages().Find(ctx, filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch messages"})
		return
	}
	defer cursor.Close(ctx)

	var messages []models.InboxMessage
	cursor.All(ctx, &messages)
	if messages == nil {
		messages = []models.InboxMessage{}
	}

	c.JSON(http.StatusOK, messages)
}

// MarkRead marks an inbox message as read.
// PATCH /inbox/:id/read
func (h *InboxHandler) MarkRead(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.DB.InboxMessages().UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"isRead": true},
	})

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ReplyToComment sends a reply to an Instagram comment.
// POST /inbox/comments/:id/reply
func (h *InboxHandler) ReplyToComment(c *gin.Context) {
	msgID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var input struct {
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var msg models.InboxMessage
	if err := h.DB.InboxMessages().FindOne(ctx, bson.M{"_id": msgID}).Decode(&msg); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
		return
	}

	accountID := msg.AccountID.Hex()
	if err := h.Instagram.ReplyToComment(ctx, msg.ExternalID, input.Text, accountID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	h.DB.InboxMessages().UpdateOne(ctx, bson.M{"_id": msgID}, bson.M{
		"$set": bson.M{"isReplied": true, "isRead": true},
	})

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ReplyToDM sends a reply to an Instagram DM.
// POST /inbox/dms/:id/reply
func (h *InboxHandler) ReplyToDM(c *gin.Context) {
	msgID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var input struct {
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var msg models.InboxMessage
	if err := h.DB.InboxMessages().FindOne(ctx, bson.M{"_id": msgID}).Decode(&msg); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
		return
	}

	accountID := msg.AccountID.Hex()
	if err := h.Instagram.SendDM(ctx, msg.SenderID, input.Text, accountID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	h.DB.InboxMessages().UpdateOne(ctx, bson.M{"_id": msgID}, bson.M{
		"$set": bson.M{"isReplied": true, "isRead": true},
	})

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetFeed fetches the authenticated user's own Instagram media feed.
// GET /inbox/feed?accountId=xxx
func (h *InboxHandler) GetFeed(c *gin.Context) {
	accountID := c.Query("accountId")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	feed, err := h.Instagram.FetchFeed(ctx, accountID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, feed)
}
