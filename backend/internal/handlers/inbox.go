package handlers

import (
	"context"
	"log"
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
	Bluesky   *services.BlueskyService
}

// ListComments returns paginated comment inbox entries across all platforms.
// GET /inbox/comments?page=1&limit=50
func (h *InboxHandler) ListComments(c *gin.Context) {
	h.listMessages(c, models.MessageTypeComment)
}

// ListDMs returns paginated DM inbox entries across all platforms.
// GET /inbox/dms?page=1&limit=50
func (h *InboxHandler) ListDMs(c *gin.Context) {
	// Sync Bluesky DMs on-demand (no webhook available)
	h.syncBlueskyDMs()
	h.listMessages(c, models.MessageTypeDM)
}

// syncBlueskyDMs fetches recent Bluesky conversations and inserts new
// incoming messages into the inbox. Only messages from other users are stored.
func (h *InboxHandler) syncBlueskyDMs() {
	if h.Bluesky == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	convos, account, err := h.Bluesky.ListConvos(ctx, "")
	if err != nil {
		log.Printf("syncBlueskyDMs: ListConvos error: %v", err)
		return
	}

	upsert := true
	for _, convo := range convos {
		if convo.LastMessage == nil || convo.LastMessage.Text == "" {
			continue
		}

		// Skip messages sent by our own account
		if convo.LastMessage.Sender.DID == account.DID {
			continue
		}

		// Find the sender profile from the conversation members
		var senderName, senderHandle string
		for _, m := range convo.Members {
			if m.DID == convo.LastMessage.Sender.DID {
				senderName = m.DisplayName
				senderHandle = m.Handle
				break
			}
		}
		displayName := senderName
		if displayName == "" {
			displayName = senderHandle
		}

		receivedAt := time.Now()
		if t, err := time.Parse(time.RFC3339, convo.LastMessage.SentAt); err == nil {
			receivedAt = t
		}

		msg := models.InboxMessage{
			Platform:    models.PlatformBluesky,
			MessageType: models.MessageTypeDM,
			ExternalID:  convo.LastMessage.ID,
			ThreadID:    convo.ID,
			SenderID:    convo.LastMessage.Sender.DID,
			SenderName:  displayName,
			Text:        convo.LastMessage.Text,
			AccountID:   account.ID,
			AccountName: account.AccountName,
			IsRead:      false,
			IsReplied:   false,
			ReceivedAt:  receivedAt,
			CreatedAt:   time.Now(),
		}

		doc, err := structToBSONDoc(msg)
		if err != nil {
			continue
		}
		h.DB.InboxMessages().UpdateOne(ctx,
			bson.M{"externalId": msg.ExternalID},
			bson.M{"$setOnInsert": doc},
			&options.UpdateOptions{Upsert: &upsert},
		)
	}
}

// backfillSenderNames attempts to resolve sender names for DM messages that
// have a senderId but no senderName. It fetches the name from Instagram and
// updates the database record so future requests don't need to re-fetch.
func (h *InboxHandler) backfillSenderNames(ctx context.Context, messages []models.InboxMessage) {
	if h.Instagram == nil {
		return
	}

	// Build a cache of account access tokens keyed by account ID
	tokenCache := map[primitive.ObjectID]string{}
	for i := range messages {
		msg := &messages[i]
		if msg.SenderName != "" || msg.SenderID == "" || msg.Platform != models.PlatformInstagram {
			continue
		}

		token, ok := tokenCache[msg.AccountID]
		if !ok {
			var account models.SocialAccount
			err := h.DB.SocialAccounts().FindOne(ctx, bson.M{
				"_id":      msg.AccountID,
				"platform": models.PlatformInstagram,
				"isActive": true,
			}).Decode(&account)
			if err != nil {
				// Try any active Instagram account as fallback
				err = h.DB.SocialAccounts().FindOne(ctx, bson.M{
					"platform": models.PlatformInstagram,
					"isActive": true,
				}).Decode(&account)
			}
			if err != nil {
				continue
			}
			token = account.AccessToken
			tokenCache[msg.AccountID] = token
		}

		name := h.Instagram.GetSenderName(ctx, msg.SenderID, token)
		if name != "" {
			msg.SenderName = name
			// Update in DB so we don't re-fetch next time
			h.DB.InboxMessages().UpdateOne(ctx,
				bson.M{"_id": msg.ID},
				bson.M{"$set": bson.M{"senderName": name}},
			)
		}
	}
}

// backfillMediaURLs fetches Instagram permalinks for comments that have a
// mediaId but no mediaUrl, and persists them in the database.
func (h *InboxHandler) backfillMediaURLs(ctx context.Context, messages []models.InboxMessage) {
	if h.Instagram == nil {
		return
	}

	tokenCache := map[primitive.ObjectID]string{}
	for i := range messages {
		msg := &messages[i]
		if msg.MediaURL != "" || msg.MediaID == "" || msg.Platform != models.PlatformInstagram {
			continue
		}

		token, ok := tokenCache[msg.AccountID]
		if !ok {
			var account models.SocialAccount
			err := h.DB.SocialAccounts().FindOne(ctx, bson.M{
				"_id":      msg.AccountID,
				"platform": models.PlatformInstagram,
				"isActive": true,
			}).Decode(&account)
			if err != nil {
				err = h.DB.SocialAccounts().FindOne(ctx, bson.M{
					"platform": models.PlatformInstagram,
					"isActive": true,
				}).Decode(&account)
			}
			if err != nil {
				continue
			}
			token = account.AccessToken
			tokenCache[msg.AccountID] = token
		}

		permalink := h.Instagram.GetMediaPermalink(ctx, msg.MediaID, token)
		if permalink != "" {
			msg.MediaURL = permalink
			h.DB.InboxMessages().UpdateOne(ctx,
				bson.M{"_id": msg.ID},
				bson.M{"$set": bson.M{"mediaUrl": permalink}},
			)
		}
	}
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
		log.Printf("InboxMessages Find error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch messages"})
		return
	}
	defer cursor.Close(ctx)

	var messages []models.InboxMessage
	if err := cursor.All(ctx, &messages); err != nil {
		log.Printf("InboxMessages cursor.All error (type=%s): %v", msgType, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode messages"})
		return
	}
	if messages == nil {
		messages = []models.InboxMessage{}
	}

	// Backfill sender names for DMs that are missing them
	if msgType == models.MessageTypeDM {
		h.backfillSenderNames(ctx, messages)
	}

	// Backfill media permalinks for comments that have a mediaId but no mediaUrl
	if msgType == models.MessageTypeComment {
		h.backfillMediaURLs(ctx, messages)
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

	accountID := ""
	if !msg.AccountID.IsZero() {
		accountID = msg.AccountID.Hex()
	}
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

	accountID := ""
	if !msg.AccountID.IsZero() {
		accountID = msg.AccountID.Hex()
	}

	switch msg.Platform {
	case models.PlatformBluesky:
		if h.Bluesky == nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Bluesky service not configured"})
			return
		}
		if msg.ThreadID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing conversation ID"})
			return
		}
		if err := h.Bluesky.SendChatMessage(ctx, msg.ThreadID, input.Text, accountID); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
	default:
		if err := h.Instagram.SendDM(ctx, msg.SenderID, input.Text, accountID); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
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
