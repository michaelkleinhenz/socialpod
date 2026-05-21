package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	teamIDRaw, _ := c.Get("teamId")
	teamIDStr, _ := teamIDRaw.(string)
	h.syncBlueskyComments(teamIDStr)
	h.listMessages(c, models.MessageTypeComment)
}

// syncBlueskyComments fetches recent Bluesky notifications for all accounts
// belonging to the given team and inserts them as comment inbox entries.
func (h *InboxHandler) syncBlueskyComments(teamIDStr string) {
	if h.Bluesky == nil || teamIDStr == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid, err := primitive.ObjectIDFromHex(teamIDStr)
	if err != nil {
		return
	}

	cursor, err := h.DB.SocialAccounts().Find(ctx, bson.M{
		"platform": models.PlatformBluesky,
		"isActive": true,
		"teamId":   tid,
	})
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	var bskyAccounts []models.SocialAccount
	if err := cursor.All(ctx, &bskyAccounts); err != nil || len(bskyAccounts) == 0 {
		return
	}

	upsert := true
	for _, acc := range bskyAccounts {
		notifications, account, fetchErr := h.Bluesky.FetchNotifications(ctx, acc.ID.Hex())
		if fetchErr != nil {
			log.Printf("syncBlueskyComments: FetchNotifications error for account %s: %v", acc.ID.Hex(), fetchErr)
			continue
		}

		for _, n := range notifications {
			if n.Reason != "reply" && n.Reason != "mention" {
				continue
			}
			if n.Record.Text == "" {
				continue
			}
			if n.Author.DID == account.DID {
				continue
			}

			receivedAt := time.Now()
			if t, parseErr := time.Parse(time.RFC3339, n.IndexedAt); parseErr == nil {
				receivedAt = t
			}

			mediaURL := ""
			if n.Record.Reply != nil && n.Record.Reply.Parent.URI != "" {
				mediaURL = bskyURIToPermalink(n.Record.Reply.Parent.URI, account.AccountName)
			}

			senderName := n.Author.DisplayName
			if senderName == "" {
				senderName = n.Author.Handle
			}

			msg := models.InboxMessage{
				Platform:    models.PlatformBluesky,
				MessageType: models.MessageTypeComment,
				TeamID:      account.TeamID,
				ExternalID:  n.URI,
				SenderID:    n.Author.DID,
				SenderName:  senderName,
				Text:        n.Record.Text,
				MediaURL:    mediaURL,
				AccountID:   account.ID,
				AccountName: account.AccountName,
				IsRead:      false,
				IsReplied:   false,
				ReceivedAt:  receivedAt,
				CreatedAt:   time.Now(),
			}

			doc, docErr := structToBSONDoc(msg)
			if docErr != nil {
				continue
			}
			h.DB.InboxMessages().UpdateOne(ctx,
				bson.M{"externalId": msg.ExternalID},
				bson.M{"$setOnInsert": doc},
				&options.UpdateOptions{Upsert: &upsert},
			)
		}
	}
}

// bskyURIToPermalink converts an AT URI to a Bluesky web permalink.
// at://did:plc:xxx/app.bsky.feed.post/rkey -> https://bsky.app/profile/did:plc:xxx/post/rkey
func bskyURIToPermalink(uri, fallbackHandle string) string {
	// at://did:plc:xxx/app.bsky.feed.post/rkey
	parts := strings.SplitN(strings.TrimPrefix(uri, "at://"), "/", 3)
	if len(parts) != 3 {
		return ""
	}
	actor := parts[0]
	rkey := strings.TrimPrefix(parts[2], "")
	// parts[1] should be "app.bsky.feed.post"
	if parts[1] != "app.bsky.feed.post" {
		return ""
	}
	return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", actor, rkey)
}

// ListDMs returns paginated DM inbox entries across all platforms.
// GET /inbox/dms?page=1&limit=50
func (h *InboxHandler) ListDMs(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	teamIDStr, _ := teamIDRaw.(string)
	h.syncBlueskyDMs(teamIDStr)
	h.listMessages(c, models.MessageTypeDM)
}

// syncBlueskyDMs fetches recent Bluesky conversations for all accounts
// belonging to the given team and inserts new incoming messages into the inbox.
func (h *InboxHandler) syncBlueskyDMs(teamIDStr string) {
	if h.Bluesky == nil || teamIDStr == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid, err := primitive.ObjectIDFromHex(teamIDStr)
	if err != nil {
		return
	}

	cursor, err := h.DB.SocialAccounts().Find(ctx, bson.M{
		"platform": models.PlatformBluesky,
		"isActive": true,
		"teamId":   tid,
	})
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	var bskyAccounts []models.SocialAccount
	if err := cursor.All(ctx, &bskyAccounts); err != nil || len(bskyAccounts) == 0 {
		return
	}

	upsert := true
	for _, acc := range bskyAccounts {
		convos, account, fetchErr := h.Bluesky.ListConvos(ctx, acc.ID.Hex())
		if fetchErr != nil {
			log.Printf("syncBlueskyDMs: ListConvos error for account %s: %v", acc.ID.Hex(), fetchErr)
			continue
		}

		log.Printf("syncBlueskyDMs: got %d convos, account DID=%s", len(convos), account.DID)

		for _, convo := range convos {
			lastMsg := convo.ParseLastMessage()
			if lastMsg == nil || lastMsg.Text == "" {
				log.Printf("syncBlueskyDMs: skipping convo %s - no last message or empty text", convo.ID)
				continue
			}

			if lastMsg.Sender.DID == account.DID {
				log.Printf("syncBlueskyDMs: skipping convo %s - own message (sender=%s)", convo.ID, lastMsg.Sender.DID)
				continue
			}

			var senderName, senderHandle string
			for _, m := range convo.Members {
				if m.DID == lastMsg.Sender.DID {
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
			if t, parseErr := time.Parse(time.RFC3339, lastMsg.SentAt); parseErr == nil {
				receivedAt = t
			}

			msg := models.InboxMessage{
				Platform:    models.PlatformBluesky,
				MessageType: models.MessageTypeDM,
				TeamID:      account.TeamID,
				ExternalID:  lastMsg.ID,
				ThreadID:    convo.ID,
				SenderID:    lastMsg.Sender.DID,
				SenderName:  displayName,
				Text:        lastMsg.Text,
				AccountID:   account.ID,
				AccountName: account.AccountName,
				IsRead:      false,
				IsReplied:   false,
				ReceivedAt:  receivedAt,
				CreatedAt:   time.Now(),
			}

			doc, docErr := structToBSONDoc(msg)
			if docErr != nil {
				continue
			}
			h.DB.InboxMessages().UpdateOne(ctx,
				bson.M{"externalId": msg.ExternalID},
				bson.M{"$setOnInsert": doc},
				&options.UpdateOptions{Upsert: &upsert},
			)
		}
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

	teamIDRaw, hasTeam := c.Get("teamId")
	if !hasTeam || teamIDRaw.(string) == "" {
		c.JSON(http.StatusOK, []models.InboxMessage{})
		return
	}
	tid, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))
	filter["teamId"] = tid

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

	teamIDRaw, hasTeam := c.Get("teamId")
	if !hasTeam || teamIDRaw.(string) == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}
	tid, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))
	msgFilter := bson.M{"_id": id, "teamId": tid}

	result, err := h.DB.InboxMessages().UpdateOne(ctx, msgFilter, bson.M{
		"$set": bson.M{"isRead": true},
	})
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
		return
	}

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

	teamIDRaw, hasTeam := c.Get("teamId")
	if !hasTeam || teamIDRaw.(string) == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}
	replyTid, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))
	msgFilter := bson.M{"_id": msgID, "teamId": replyTid}

	var msg models.InboxMessage
	if err := h.DB.InboxMessages().FindOne(ctx, msgFilter).Decode(&msg); err != nil {
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
		// Resolve CID for the post we're replying to
		cid, err := h.Bluesky.ResolveURI(ctx, msg.ExternalID, accountID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to resolve post: " + err.Error()})
			return
		}
		_, err = h.Bluesky.PostReply(ctx, input.Text, msg.ExternalID, cid, accountID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
	default:
		if err := h.Instagram.ReplyToComment(ctx, msg.ExternalID, input.Text, accountID); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
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

	dmTeamIDRaw, dmHasTeam := c.Get("teamId")
	if !dmHasTeam || dmTeamIDRaw.(string) == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}
	dmTid, _ := primitive.ObjectIDFromHex(dmTeamIDRaw.(string))
	msgFilter := bson.M{"_id": msgID, "teamId": dmTid}

	var msg models.InboxMessage
	if err := h.DB.InboxMessages().FindOne(ctx, msgFilter).Decode(&msg); err != nil {
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

// GetFeed fetches the authenticated user's own feed for a given account.
// GET /inbox/feed?accountId=xxx&platform=instagram
func (h *InboxHandler) GetFeed(c *gin.Context) {
	accountID := c.Query("accountId")
	platform := c.Query("platform")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Verify the requested account belongs to the caller's team.
	if accountID != "" {
		accObjID, err := primitive.ObjectIDFromHex(accountID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
			return
		}
		feedTeamIDRaw, feedHasTeam := c.Get("teamId")
		if !feedHasTeam || feedTeamIDRaw.(string) == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}
		feedTid, err := primitive.ObjectIDFromHex(feedTeamIDRaw.(string))
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}
		accFilter := bson.M{"_id": accObjID, "isActive": true, "teamId": feedTid}
		var acc models.SocialAccount
		if err := h.DB.SocialAccounts().FindOne(ctx, accFilter).Decode(&acc); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Account not found or access denied"})
			return
		}
	}

	switch platform {
	case string(models.PlatformBluesky):
		if h.Bluesky == nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Bluesky service not configured"})
			return
		}
		feed, err := h.Bluesky.FetchFeed(ctx, accountID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, feed)
	default:
		feed, err := h.Instagram.FetchFeed(ctx, accountID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, feed)
	}
}
