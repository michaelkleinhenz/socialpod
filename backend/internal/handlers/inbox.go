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
)

type InboxHandler struct {
	DB        *database.MongoDB
	Instagram *services.InstagramService
	Bluesky   *services.BlueskyService
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
