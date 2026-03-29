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

type AdminHandler struct {
	DB        *database.MongoDB
	Instagram *services.InstagramService
}

func (h *AdminHandler) ListAccounts(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.SocialAccounts().Find(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch accounts"})
		return
	}
	defer cursor.Close(ctx)

	var accounts []models.SocialAccount
	cursor.All(ctx, &accounts)
	if accounts == nil {
		accounts = []models.SocialAccount{}
	}

	c.JSON(http.StatusOK, accounts)
}

type AddBlueskyInput struct {
	Handle      string `json:"handle" binding:"required"`
	AppPassword string `json:"appPassword" binding:"required"`
	PDSHost     string `json:"pdsHost"`
}

func (h *AdminHandler) AddBlueskyAccount(c *gin.Context) {
	var input AddBlueskyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pdsHost := input.PDSHost
	if pdsHost == "" {
		pdsHost = "https://bsky.social"
	}

	account := models.SocialAccount{
		Platform:    models.PlatformBluesky,
		AccountName: input.Handle,
		DisplayName: input.Handle,
		AppPassword: input.AppPassword,
		PDSHost:     pdsHost,
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.DB.SocialAccounts().InsertOne(ctx, account)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add account"})
		return
	}

	account.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, account)
}

func (h *AdminHandler) DeleteAccount(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.DB.SocialAccounts().DeleteOne(ctx, bson.M{"_id": id})
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Account deleted"})
}

func (h *AdminHandler) ToggleAccount(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var account models.SocialAccount
	err = h.DB.SocialAccounts().FindOne(ctx, bson.M{"_id": id}).Decode(&account)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	h.DB.SocialAccounts().UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"isActive": !account.IsActive, "updatedAt": time.Now()},
	})

	account.IsActive = !account.IsActive
	c.JSON(http.StatusOK, account)
}

func (h *AdminHandler) GetSettings(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var settings models.AppSettings
	err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	if err != nil {
		settings = models.AppSettings{
			AppURL:          "http://localhost:3000",
			DefaultPostTime: "09:00",
			AutoPublish:     false,
		}
	}

	c.JSON(http.StatusOK, settings)
}

type UpdateSettingsInput struct {
	AppURL             *string `json:"appUrl,omitempty"`
	InstagramAppID     *string `json:"instagramAppId,omitempty"`
	InstagramAppSecret *string `json:"instagramAppSecret,omitempty"`
	DefaultPostTime    *string `json:"defaultPostTime,omitempty"`
	AutoPublish        *bool   `json:"autoPublish,omitempty"`
}

func (h *AdminHandler) UpdateSettings(c *gin.Context) {
	var input UpdateSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{"updatedAt": time.Now()}
	if input.AppURL != nil {
		update["appUrl"] = *input.AppURL
	}
	if input.InstagramAppID != nil {
		update["instagramAppId"] = *input.InstagramAppID
	}
	if input.InstagramAppSecret != nil {
		update["instagramAppSecret"] = *input.InstagramAppSecret
	}
	if input.DefaultPostTime != nil {
		update["defaultPostTime"] = *input.DefaultPostTime
	}
	if input.AutoPublish != nil {
		update["autoPublish"] = *input.AutoPublish
	}

	upsert := true
	h.DB.Settings().UpdateOne(ctx, bson.M{}, bson.M{"$set": update}, &options.UpdateOptions{Upsert: &upsert})

	var settings models.AppSettings
	h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	c.JSON(http.StatusOK, settings)
}

func (h *AdminHandler) InstagramAuthURL(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var settings models.AppSettings
	err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	if err != nil || settings.InstagramAppID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Instagram App ID not configured in settings"})
		return
	}

	redirectURI := settings.AppURL + "/api/auth/instagram/callback"
	authURL := "https://api.instagram.com/oauth/authorize" +
		"?client_id=" + settings.InstagramAppID +
		"&redirect_uri=" + redirectURI +
		"&scope=instagram_business_basic,instagram_business_content_publish,instagram_business_manage_messages" +
		"&response_type=code" +
		"&state=instagram_auth"

	c.JSON(http.StatusOK, gin.H{"url": authURL})
}

func (h *AdminHandler) InstagramCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusFound, "/admin/accounts?error=no_code")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var settings models.AppSettings
	err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/accounts?error=no_settings")
		return
	}

	redirectURI := settings.AppURL + "/api/auth/instagram/callback"
	account, err := h.Instagram.ExchangeCodeForToken(ctx, code, settings.InstagramAppID, settings.InstagramAppSecret, redirectURI)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/accounts?error="+err.Error())
		return
	}

	account.CreatedAt = time.Now()
	account.UpdatedAt = time.Now()

	result, err := h.DB.SocialAccounts().InsertOne(ctx, account)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/accounts?error=save_failed")
		return
	}

	_ = result
	c.Redirect(http.StatusFound, "/admin/accounts?instagram=connected")
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.Users().Find(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}
	defer cursor.Close(ctx)

	var users []models.User
	cursor.All(ctx, &users)
	if users == nil {
		users = []models.User{}
	}

	c.JSON(http.StatusOK, users)
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	userID, _ := c.Get("userId")
	if userID.(string) == c.Param("id") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete yourself"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.DB.Users().DeleteOne(ctx, bson.M{"_id": id})
	h.DB.Posts().DeleteMany(ctx, bson.M{"userId": id})

	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}
