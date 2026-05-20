package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"
	"socialmedia/internal/services"

	"log"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

// structToBSONDoc marshals a struct into a bson.D, honouring all bson struct
// tags including omitempty. Using a raw struct as a value inside bson.M{}
// skips omitempty, so this helper is needed for $setOnInsert payloads.
func structToBSONDoc(v interface{}) (bson.D, error) {
	data, err := bson.Marshal(v)
	if err != nil {
		return nil, err
	}
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

type AdminHandler struct {
	DB        *database.MongoDB
	Bluesky   *services.BlueskyService
	Instagram *services.InstagramService
	Twitter   *services.TwitterService
	Mastodon  *services.MastodonService
	Threads   *services.ThreadsService
	LinkedIn  *services.LinkedInService
	UploadDir string
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
	if err := cursor.All(ctx, &accounts); err != nil {
		log.Printf("ListAccounts cursor.All error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode accounts"})
		return
	}
	if accounts == nil {
		accounts = []models.SocialAccount{}
	}

	c.JSON(http.StatusOK, accounts)
}

// ListActiveAccounts returns active accounts scoped to the user's team.
// Global admins see all active accounts; users without a team see none.
func (h *AdminHandler) ListActiveAccounts(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"isActive": true}

	isAdmin, _ := c.Get("isAdmin")
	if !isAdmin.(bool) {
		teamIDRaw, hasTeam := c.Get("teamId")
		if !hasTeam || teamIDRaw.(string) == "" {
			c.JSON(http.StatusOK, []models.SocialAccount{})
			return
		}
		tid, err := primitive.ObjectIDFromHex(teamIDRaw.(string))
		if err != nil {
			c.JSON(http.StatusOK, []models.SocialAccount{})
			return
		}
		filter["teamId"] = tid
	}

	cursor, err := h.DB.SocialAccounts().Find(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch accounts"})
		return
	}
	defer cursor.Close(ctx)

	var accounts []models.SocialAccount
	if err := cursor.All(ctx, &accounts); err != nil {
		log.Printf("ListActiveAccounts cursor.All error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode accounts"})
		return
	}
	if accounts == nil {
		accounts = []models.SocialAccount{}
	}

	c.JSON(http.StatusOK, accounts)
}

type AddBlueskyInput struct {
	Handle      string `json:"handle" binding:"required"`
	AppPassword string `json:"appPassword" binding:"required"`
	PDSHost     string `json:"pdsHost"`
	TeamID      string `json:"teamId"` // admin can assign to a specific team
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

	if input.TeamID != "" {
		if tid, err := primitive.ObjectIDFromHex(input.TeamID); err == nil {
			account.TeamID = &tid
		}
	}

	// Fetch display name and avatar from Bluesky profile
	if h.Bluesky != nil {
		if dn, av, err := h.Bluesky.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
			}
			account.AvatarURL = av
		}
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

type AddTwitterInput struct {
	ConsumerKey       string `json:"consumerKey" binding:"required"`
	ConsumerSecret    string `json:"consumerSecret" binding:"required"`
	AccessToken       string `json:"accessToken" binding:"required"`
	AccessTokenSecret string `json:"accessTokenSecret" binding:"required"`
	TeamID            string `json:"teamId"`
}

func (h *AdminHandler) AddTwitterAccount(c *gin.Context) {
	var input AddTwitterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := models.SocialAccount{
		Platform:          models.PlatformTwitter,
		AccountName:       "twitter",
		DisplayName:       "Twitter Account",
		ConsumerKey:       input.ConsumerKey,
		ConsumerSecret:    input.ConsumerSecret,
		AccessToken:       input.AccessToken,
		AccessTokenSecret: input.AccessTokenSecret,
		IsActive:          true,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if input.TeamID != "" {
		if tid, err := primitive.ObjectIDFromHex(input.TeamID); err == nil {
			account.TeamID = &tid
		}
	}

	if h.Twitter != nil {
		if dn, username, av, err := h.Twitter.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
			}
			if username != "" {
				account.AccountName = username
			}
			account.AvatarURL = av
		}
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

type AddMastodonInput struct {
	Instance    string `json:"instance" binding:"required"`
	AccessToken string `json:"accessToken" binding:"required"`
	TeamID      string `json:"teamId"`
}

func (h *AdminHandler) AddMastodonAccount(c *gin.Context) {
	var input AddMastodonInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := models.SocialAccount{
		Platform:         models.PlatformMastodon,
		AccountName:      input.Instance,
		DisplayName:      input.Instance,
		AccessToken:      input.AccessToken,
		MastodonInstance: input.Instance,
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if input.TeamID != "" {
		if tid, err := primitive.ObjectIDFromHex(input.TeamID); err == nil {
			account.TeamID = &tid
		}
	}

	if h.Mastodon != nil {
		if dn, av, err := h.Mastodon.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
			}
			account.AvatarURL = av
		}
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

type AddThreadsInput struct {
	AccessToken   string `json:"accessToken" binding:"required"`
	ThreadsUserID string `json:"threadsUserId"` // optional; fetched from API if omitted
	TeamID        string `json:"teamId"`
}

func (h *AdminHandler) AddThreadsAccount(c *gin.Context) {
	var input AddThreadsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := models.SocialAccount{
		Platform:      models.PlatformThreads,
		AccountName:   "threads",
		DisplayName:   "Threads Account",
		AccessToken:   input.AccessToken,
		ThreadsUserID: input.ThreadsUserID,
		IsActive:      true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if input.TeamID != "" {
		if tid, err := primitive.ObjectIDFromHex(input.TeamID); err == nil {
			account.TeamID = &tid
		}
	}

	if h.Threads != nil {
		if dn, av, err := h.Threads.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
				account.AccountName = dn
			}
			account.AvatarURL = av
		}
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

type AddLinkedInInput struct {
	AccessToken       string `json:"accessToken" binding:"required"`
	LinkedInPersonURN string `json:"linkedinPersonUrn"` // optional; fetched from /me if omitted
	TeamID            string `json:"teamId"`
}

func (h *AdminHandler) AddLinkedInAccount(c *gin.Context) {
	var input AddLinkedInInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := models.SocialAccount{
		Platform:          models.PlatformLinkedIn,
		AccountName:       "linkedin",
		DisplayName:       "LinkedIn Account",
		AccessToken:       input.AccessToken,
		LinkedInPersonURN: input.LinkedInPersonURN,
		IsActive:          true,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if input.TeamID != "" {
		if tid, err := primitive.ObjectIDFromHex(input.TeamID); err == nil {
			account.TeamID = &tid
		}
	}

	if h.LinkedIn != nil {
		if dn, av, err := h.LinkedIn.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
				account.AccountName = dn
			}
			account.AvatarURL = av
		}
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

func (h *AdminHandler) GetPublicSettings(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var settings models.AppSettings
	h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)

	c.JSON(http.StatusOK, gin.H{
		"adobeExpressClientId": settings.AdobeExpressClientID,
		"imprintHtml":          settings.ImprintHTML,
		"cookieBannerEnabled":  settings.CookieBannerEnabled,
		"cookieBannerText":     settings.CookieBannerText,
		"openRouterEnabled":    settings.OpenRouterAPIKey != "",
	})
}

func (h *AdminHandler) GetSettings(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var settings models.AppSettings
	err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	if err != nil {
		settings = models.AppSettings{
			AppURL: "http://localhost:3000",
		}
	}

	// Add hasOpenRouterKey so the admin UI can show key status without leaking it.
	type settingsResponse struct {
		models.AppSettings
		HasOpenRouterKey bool `json:"hasOpenRouterKey"`
		HasBGGAPIToken   bool `json:"hasBggApiToken"`
	}
	c.JSON(http.StatusOK, settingsResponse{
		AppSettings:      settings,
		HasOpenRouterKey: settings.OpenRouterAPIKey != "",
		HasBGGAPIToken:   settings.BGGAPIToken != "",
	})
}

type UpdateSettingsInput struct {
	AppURL                *string `json:"appUrl,omitempty"`
	InstagramAppID        *string `json:"instagramAppId,omitempty"`
	InstagramAppSecret    *string `json:"instagramAppSecret,omitempty"`
	WebhookVerifyToken    *string `json:"webhookVerifyToken,omitempty"`
	AdobeExpressClientID  *string `json:"adobeExpressClientId,omitempty"`
	AllowSelfRegistration *bool   `json:"allowSelfRegistration,omitempty"`
	ImprintHTML           *string `json:"imprintHtml,omitempty"`
	CookieBannerEnabled   *bool   `json:"cookieBannerEnabled,omitempty"`
	CookieBannerText      *string `json:"cookieBannerText,omitempty"`
	OpenRouterAPIKey      *string `json:"openRouterApiKey,omitempty"`
	OpenRouterModel       *string `json:"openRouterModel,omitempty"`
	AILanguage            *string `json:"aiLanguage,omitempty"`
	BGGAPIToken           *string `json:"bggApiToken,omitempty"`
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
	if input.AllowSelfRegistration != nil {
		update["allowSelfRegistration"] = *input.AllowSelfRegistration
	}
	if input.WebhookVerifyToken != nil {
		update["webhookVerifyToken"] = *input.WebhookVerifyToken
	}
	if input.AdobeExpressClientID != nil {
		update["adobeExpressClientId"] = *input.AdobeExpressClientID
	}
	if input.ImprintHTML != nil {
		update["imprintHtml"] = *input.ImprintHTML
	}
	if input.CookieBannerEnabled != nil {
		update["cookieBannerEnabled"] = *input.CookieBannerEnabled
	}
	if input.CookieBannerText != nil {
		update["cookieBannerText"] = *input.CookieBannerText
	}
	if input.OpenRouterAPIKey != nil {
		update["openRouterApiKey"] = *input.OpenRouterAPIKey
	}
	if input.OpenRouterModel != nil {
		update["openRouterModel"] = *input.OpenRouterModel
	}
	if input.AILanguage != nil {
		update["aiLanguage"] = *input.AILanguage
	}
	if input.BGGAPIToken != nil {
		update["bggApiToken"] = *input.BGGAPIToken
	}

	upsert := true
	h.DB.Settings().UpdateOne(ctx, bson.M{}, bson.M{"$set": update}, &options.UpdateOptions{Upsert: &upsert})

	var settings models.AppSettings
	h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	c.JSON(http.StatusOK, settings)
}

func (h *AdminHandler) instagramAuthURL(ctx context.Context, state string) (string, error) {
	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil {
		return "", fmt.Errorf("settings not found")
	}
	if settings.InstagramAppID == "" {
		return "", fmt.Errorf("Instagram App ID not configured in settings")
	}
	redirectURI := settings.AppURL + "/api/auth/instagram/callback"
	authURL := "https://api.instagram.com/oauth/authorize" +
		"?client_id=" + settings.InstagramAppID +
		"&redirect_uri=" + redirectURI +
		"&scope=instagram_business_basic,instagram_business_content_publish,instagram_business_manage_messages,instagram_business_manage_comments" +
		"&response_type=code" +
		"&state=" + state
	return authURL, nil
}

func (h *AdminHandler) InstagramAuthURL(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url, err := h.instagramAuthURL(ctx, "instagram_auth")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url})
}

func (h *AdminHandler) InstagramCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	// Parse optional team ID from state (format: "instagram_auth:TEAMID" or "instagram_auth")
	var teamID *primitive.ObjectID
	var successRedirect, errorRedirect string
	if strings.HasPrefix(state, "instagram_auth:") {
		rawID := strings.TrimPrefix(state, "instagram_auth:")
		if tid, err := primitive.ObjectIDFromHex(rawID); err == nil {
			teamID = &tid
		}
		successRedirect = "/team/accounts?instagram=connected"
		errorRedirect = "/team/accounts?error="
	} else {
		successRedirect = "/admin/accounts?instagram=connected"
		errorRedirect = "/admin/accounts?error="
	}

	if code == "" {
		c.Redirect(http.StatusFound, errorRedirect+"no_code")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var settings models.AppSettings
	err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	if err != nil {
		c.Redirect(http.StatusFound, errorRedirect+"no_settings")
		return
	}

	redirectURI := settings.AppURL + "/api/auth/instagram/callback"
	account, err := h.Instagram.ExchangeCodeForToken(ctx, code, settings.InstagramAppID, settings.InstagramAppSecret, redirectURI)
	if err != nil {
		c.Redirect(http.StatusFound, errorRedirect+err.Error())
		return
	}

	account.TeamID = teamID
	account.CreatedAt = time.Now()
	account.UpdatedAt = time.Now()

	result, err := h.DB.SocialAccounts().InsertOne(ctx, account)
	if err != nil {
		c.Redirect(http.StatusFound, errorRedirect+"save_failed")
		return
	}

	_ = result
	c.Redirect(http.StatusFound, successRedirect)
}

func (h *AdminHandler) InstagramWebhookVerify(c *gin.Context) {
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")

	if mode != "subscribe" || token == "" || challenge == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var settings models.AppSettings
	err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	if err != nil || settings.WebhookVerifyToken == "" || settings.WebhookVerifyToken != token {
		c.JSON(http.StatusForbidden, gin.H{"error": "Verification failed"})
		return
	}

	c.String(http.StatusOK, challenge)
}

// metaTimestamp handles Meta webhook timestamps delivered as either a JSON
// number (1234567890) or a quoted string ("1234567890").
type metaTimestamp int64

func (t *metaTimestamp) UnmarshalJSON(data []byte) error {
	var n int64
	if err := json.Unmarshal(data, &n); err == nil {
		*t = metaTimestamp(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*t = metaTimestamp(n)
	return nil
}

// igWebhookPayload is the top-level Instagram webhook payload.
type igWebhookPayload struct {
	Object string           `json:"object"`
	Entry  []igWebhookEntry `json:"entry"`
}

type igWebhookEntry struct {
	ID        string             `json:"id"`
	Time      int64              `json:"time"`
	Changes   []igWebhookChange  `json:"changes"`
	Messaging []igWebhookMessaging `json:"messaging"`
}

type igWebhookChange struct {
	Field string              `json:"field"`
	Value igWebhookChangeVal  `json:"value"`
}

type igWebhookChangeVal struct {
	// Comment fields
	From      *igWebhookUser `json:"from"`
	Media     *igWebhookMedia `json:"media"`
	ID        string          `json:"id"`
	Text      string          `json:"text"`
	Timestamp metaTimestamp   `json:"timestamp"`
	// DM fields (field: "messages")
	Sender    *igWebhookSender    `json:"sender"`
	Recipient *igWebhookRecipient `json:"recipient"`
	Message   *igWebhookMsg       `json:"message"`
}

type igWebhookUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type igWebhookMedia struct {
	ID              string `json:"id"`
	MediaProductType string `json:"media_product_type"`
}

type igWebhookSender struct {
	ID string `json:"id"`
}

type igWebhookRecipient struct {
	ID string `json:"id"`
}

type igWebhookMsg struct {
	MID  string `json:"mid"`
	Text string `json:"text"`
}

type igWebhookMessaging struct {
	Sender    igWebhookSender    `json:"sender"`
	Recipient igWebhookRecipient `json:"recipient"`
	Timestamp metaTimestamp      `json:"timestamp"`
	Message   *igWebhookMsg      `json:"message"`
}

func (h *AdminHandler) InstagramWebhookEvent(c *gin.Context) {
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	log.Printf("Instagram webhook raw body: %s", string(bodyBytes))

	var payload igWebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		log.Printf("Instagram webhook parse error: %v", err)
		c.JSON(http.StatusOK, gin.H{"status": "received"})
		return
	}
	log.Printf("Instagram webhook parsed: object=%q entries=%d", payload.Object, len(payload.Entry))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, entry := range payload.Entry {
		// Resolve the social account by IG user ID (entry.ID contains the IG user ID)
		var account models.SocialAccount
		accountFound := h.DB.SocialAccounts().FindOne(ctx, bson.M{
			"platform": models.PlatformInstagram,
			"igUserId": entry.ID,
			"isActive": true,
		}).Decode(&account) == nil

		// Process changes (comments and DM changes)
		for _, change := range entry.Changes {
			switch change.Field {
			case "comments":
				h.processIGComment(ctx, change.Value, &account, accountFound)
			case "messages":
				h.processIGDMChange(ctx, change.Value, &account, accountFound)
			}
		}

		// Process messaging array (Messenger Platform style DM delivery)
		for _, msg := range entry.Messaging {
			h.processIGMessaging(ctx, msg, &account, accountFound)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func (h *AdminHandler) processIGComment(ctx context.Context, val igWebhookChangeVal, account *models.SocialAccount, accountFound bool) {
	if val.ID == "" {
		return
	}

	// Skip comments made by the account itself (e.g. our own replies)
	if accountFound && val.From != nil && val.From.ID == account.IGUserID {
		return
	}

	receivedAt := time.Now()
	if val.Timestamp != 0 {
		receivedAt = time.Unix(int64(val.Timestamp), 0)
	}
	msg := models.InboxMessage{
		Platform:    models.PlatformInstagram,
		MessageType: models.MessageTypeComment,
		ExternalID:  val.ID,
		Text:        val.Text,
		IsRead:      false,
		IsReplied:   false,
		ReceivedAt:  receivedAt,
		CreatedAt:   time.Now(),
	}

	if val.From != nil {
		msg.SenderID = val.From.ID
		msg.SenderName = val.From.Username
	}
	if val.Media != nil {
		msg.MediaID = val.Media.ID
		if accountFound && val.Media.ID != "" {
			msg.MediaURL = h.Instagram.GetMediaPermalink(ctx, val.Media.ID, account.AccessToken)
		}
	}
	if accountFound {
		msg.AccountID = account.ID
		msg.AccountName = account.AccountName
		msg.TeamID = account.TeamID
	}

	// Upsert by externalId to avoid duplicates
	doc, err := structToBSONDoc(msg)
	if err != nil {
		log.Printf("inbox upsert marshal error: %v", err)
		return
	}
	upsert := true
	h.DB.InboxMessages().UpdateOne(ctx,
		bson.M{"externalId": msg.ExternalID},
		bson.M{"$setOnInsert": doc},
		&options.UpdateOptions{Upsert: &upsert},
	)
}

func (h *AdminHandler) processIGDMChange(ctx context.Context, val igWebhookChangeVal, account *models.SocialAccount, accountFound bool) {
	log.Printf("processIGDMChange: sender=%v message=%v timestamp=%d", val.Sender, val.Message, val.Timestamp)
	if val.Message == nil || val.Message.MID == "" {
		log.Printf("processIGDMChange: skipping - message nil or empty MID")
		return
	}

	// Skip echo (messages sent by the account itself)
	if accountFound && val.Sender != nil && val.Sender.ID == account.IGUserID {
		return
	}
	// Also skip if recipient is not us (outgoing message echo)
	if accountFound && val.Recipient != nil && val.Recipient.ID != "" && val.Recipient.ID != account.IGUserID {
		return
	}

	dmReceivedAt := time.Now()
	if val.Timestamp != 0 {
		dmReceivedAt = time.Unix(int64(val.Timestamp), 0)
	}
	msg := models.InboxMessage{
		Platform:    models.PlatformInstagram,
		MessageType: models.MessageTypeDM,
		ExternalID:  val.Message.MID,
		Text:        val.Message.Text,
		IsRead:      false,
		IsReplied:   false,
		ReceivedAt:  dmReceivedAt,
		CreatedAt:   time.Now(),
	}
	if val.Sender != nil {
		msg.SenderID = val.Sender.ID
		if accountFound && val.Sender.ID != "" {
			msg.SenderName = h.Instagram.GetSenderName(ctx, val.Sender.ID, account.AccessToken)
		}
	}
	if accountFound {
		msg.AccountID = account.ID
		msg.AccountName = account.AccountName
		msg.TeamID = account.TeamID
	}

	doc, err := structToBSONDoc(msg)
	if err != nil {
		log.Printf("inbox upsert marshal error: %v", err)
		return
	}
	upsert := true
	h.DB.InboxMessages().UpdateOne(ctx,
		bson.M{"externalId": msg.ExternalID},
		bson.M{"$setOnInsert": doc},
		&options.UpdateOptions{Upsert: &upsert},
	)
}

func (h *AdminHandler) processIGMessaging(ctx context.Context, messaging igWebhookMessaging, account *models.SocialAccount, accountFound bool) {
	log.Printf("processIGMessaging: sender=%s mid=%v timestamp=%d", messaging.Sender.ID, messaging.Message, messaging.Timestamp)
	if messaging.Message == nil || messaging.Message.MID == "" {
		log.Printf("processIGMessaging: skipping - message nil or empty MID")
		return
	}

	// Skip echo (messages sent by the account itself)
	if accountFound && messaging.Sender.ID == account.IGUserID {
		return
	}
	// Also skip if recipient is not us (outgoing message echo)
	if accountFound && messaging.Recipient.ID != "" && messaging.Recipient.ID != account.IGUserID {
		return
	}

	msg := models.InboxMessage{
		Platform:    models.PlatformInstagram,
		MessageType: models.MessageTypeDM,
		ExternalID:  messaging.Message.MID,
		SenderID:    messaging.Sender.ID,
		Text:        messaging.Message.Text,
		IsRead:      false,
		IsReplied:   false,
		ReceivedAt:  time.Unix(int64(messaging.Timestamp)/1000, 0),
		CreatedAt:   time.Now(),
	}
	if accountFound {
		msg.AccountID = account.ID
		msg.AccountName = account.AccountName
		msg.TeamID = account.TeamID
		if messaging.Sender.ID != "" {
			msg.SenderName = h.Instagram.GetSenderName(ctx, messaging.Sender.ID, account.AccessToken)
		}
	}

	doc, err := structToBSONDoc(msg)
	if err != nil {
		log.Printf("inbox upsert marshal error: %v", err)
		return
	}
	upsert := true
	h.DB.InboxMessages().UpdateOne(ctx,
		bson.M{"externalId": msg.ExternalID},
		bson.M{"$setOnInsert": doc},
		&options.UpdateOptions{Upsert: &upsert},
	)
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
	if err := cursor.All(ctx, &users); err != nil {
		log.Printf("ListUsers cursor.All error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode users"})
		return
	}
	if users == nil {
		users = []models.User{}
	}

	c.JSON(http.StatusOK, users)
}

type CreateUserInput struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	Name        string `json:"name" binding:"required"`
	IsAdmin     bool   `json:"isAdmin"`
	IsTeamAdmin bool   `json:"isTeamAdmin"`
}

func (h *AdminHandler) CreateUser(c *gin.Context) {
	var input CreateUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, _ := h.DB.Users().CountDocuments(ctx, bson.M{"email": input.Email})
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already registered"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user := models.User{
		Email:       input.Email,
		Password:    string(hash),
		Name:        input.Name,
		IsAdmin:     input.IsAdmin,
		IsTeamAdmin: input.IsTeamAdmin,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result, err := h.DB.Users().InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	user.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, user)
}

func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var input struct {
		IsAdmin     *bool `json:"isAdmin,omitempty"`
		IsTeamAdmin *bool `json:"isTeamAdmin,omitempty"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{"updatedAt": time.Now()}
	if input.IsAdmin != nil {
		update["isAdmin"] = *input.IsAdmin
	}
	if input.IsTeamAdmin != nil {
		update["isTeamAdmin"] = *input.IsTeamAdmin
	}

	result, err := h.DB.Users().UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var user models.User
	h.DB.Users().FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	c.JSON(http.StatusOK, user)
}

func (h *AdminHandler) AssignAccountTeam(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var input struct {
		TeamID *string `json:"teamId"`
	}
	c.ShouldBindJSON(&input)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var updateDoc bson.M
	if input.TeamID == nil || *input.TeamID == "" {
		updateDoc = bson.M{
			"$unset": bson.M{"teamId": ""},
			"$set":   bson.M{"updatedAt": time.Now()},
		}
	} else {
		tid, err := primitive.ObjectIDFromHex(*input.TeamID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
			return
		}
		updateDoc = bson.M{"$set": bson.M{"teamId": tid, "updatedAt": time.Now()}}
	}

	result, err := h.DB.SocialAccounts().UpdateOne(ctx, bson.M{"_id": id}, updateDoc)
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	var account models.SocialAccount
	h.DB.SocialAccounts().FindOne(ctx, bson.M{"_id": id}).Decode(&account)
	c.JSON(http.StatusOK, account)
}

// Teams

func (h *AdminHandler) ListTeams(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.Teams().Find(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch teams"})
		return
	}
	defer cursor.Close(ctx)

	var teams []models.Team
	if err := cursor.All(ctx, &teams); err != nil {
		log.Printf("ListTeams cursor.All error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode teams"})
		return
	}
	if teams == nil {
		teams = []models.Team{}
	}

	// Attach member list for each team
	type teamWithMembers struct {
		models.Team
		Members []models.User `json:"members"`
	}
	var result []teamWithMembers
	for _, t := range teams {
		cur, _ := h.DB.Users().Find(ctx, bson.M{"teamId": t.ID})
		var members []models.User
		if cur != nil {
			cur.All(ctx, &members)
			cur.Close(ctx)
		}
		if members == nil {
			members = []models.User{}
		}
		result = append(result, teamWithMembers{Team: t, Members: members})
	}
	if result == nil {
		result = []teamWithMembers{}
	}

	c.JSON(http.StatusOK, result)
}

type CreateTeamInput struct {
	Name string `json:"name" binding:"required"`
}

func (h *AdminHandler) CreateTeam(c *gin.Context) {
	var input CreateTeamInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	team := models.Team{
		Name:      input.Name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.DB.Teams().InsertOne(ctx, team)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create team"})
		return
	}

	team.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, team)
}

// TeamAdminCreateTeam lets a team admin create their own team when they don't
// belong to one yet. Limited to one team: the calling user is automatically
// added as the first member.
func (h *AdminHandler) TeamAdminCreateTeam(c *gin.Context) {
	userIDStr, _ := c.Get("userId")
	userID, err := primitive.ObjectIDFromHex(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var caller models.User
	if err := h.DB.Users().FindOne(ctx, bson.M{"_id": userID}).Decode(&caller); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		return
	}
	if caller.TeamID != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "You are already a member of a team"})
		return
	}

	var input CreateTeamInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	team := models.Team{
		Name:      input.Name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result, err := h.DB.Teams().InsertOne(ctx, team)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create team"})
		return
	}

	team.ID = result.InsertedID.(primitive.ObjectID)

	h.DB.Users().UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{"teamId": team.ID, "updatedAt": time.Now()},
	})

	c.JSON(http.StatusCreated, team)
}

func (h *AdminHandler) DeleteTeam(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Remove team assignment and team admin flag from all members
	h.DB.Users().UpdateMany(ctx, bson.M{"teamId": id}, bson.M{
		"$unset": bson.M{"teamId": ""},
		"$set":   bson.M{"isTeamAdmin": false, "updatedAt": time.Now()},
	})

	result, err := h.DB.Teams().DeleteOne(ctx, bson.M{"_id": id})
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Team deleted"})
}

type SetTeamMembersInput struct {
	UserIDs []string `json:"userIds" binding:"required"`
}

func (h *AdminHandler) SetTeamMembers(c *gin.Context) {
	teamID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var input SetTeamMembersInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify team exists
	count, _ := h.DB.Teams().CountDocuments(ctx, bson.M{"_id": teamID})
	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Remove all current members from this team and clear their team admin flag
	h.DB.Users().UpdateMany(ctx, bson.M{"teamId": teamID}, bson.M{
		"$unset": bson.M{"teamId": ""},
		"$set":   bson.M{"isTeamAdmin": false, "updatedAt": time.Now()},
	})

	// Add new members
	for _, uid := range input.UserIDs {
		if objID, err := primitive.ObjectIDFromHex(uid); err == nil {
			h.DB.Users().UpdateOne(ctx, bson.M{"_id": objID}, bson.M{
				"$set": bson.M{"teamId": teamID, "updatedAt": time.Now()},
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Team members updated"})
}

func (h *AdminHandler) GenerateTeamToken(c *gin.Context) {
	teamID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	bytes := make([]byte, 32)
	rand.Read(bytes)
	apiToken := "st_" + hex.EncodeToString(bytes)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.DB.Teams().UpdateOne(ctx, bson.M{"_id": teamID}, bson.M{
		"$set": bson.M{"apiToken": apiToken, "updatedAt": time.Now()},
	})
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"apiToken": apiToken})
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

func (h *AdminHandler) GenerateText(c *gin.Context) {
	var input struct {
		Prompt    string   `json:"prompt" binding:"required"`
		Platforms []string `json:"platforms,omitempty"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil || settings.OpenRouterAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OpenRouter is not configured"})
		return
	}

	model := settings.OpenRouterModel
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	systemPrompt := "You are a social media copywriter. The user gives you a prompt (which may include URLs for context). Write a ready-to-post social media post based on the prompt. Reply with ONLY the post text, no quotes, no commentary, no labels."
	if len(input.Platforms) > 0 {
		var platforms []models.Platform
		for _, p := range input.Platforms {
			platforms = append(platforms, models.Platform(p))
		}
		limit := contentLimit(platforms)
		systemPrompt += fmt.Sprintf(" The post must fit within %d characters.", limit)
	}
	if settings.AILanguage != "" {
		systemPrompt += fmt.Sprintf(" Write in %s.", settings.AILanguage)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": input.Prompt},
		},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+settings.OpenRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to reach OpenRouter: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("OpenRouter returned %d: %s", resp.StatusCode, string(body))})
		return
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Invalid response from OpenRouter"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"text": result.Choices[0].Message.Content})
}

func (h *AdminHandler) DashboardInsights(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil || settings.OpenRouterAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OpenRouter is not configured"})
		return
	}

	// Gather posts (scoped to team or user)
	cursor, err := h.DB.Posts().Find(ctx, postFilter(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load posts"})
		return
	}
	var posts []models.Post
	if err := cursor.All(ctx, &posts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode posts"})
		return
	}

	// Gather accounts (scoped to team when applicable)
	accountFilter := bson.M{"isActive": true}
	if teamIDRaw, hasTeam := c.Get("teamId"); hasTeam {
		tid, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))
		accountFilter["teamId"] = tid
	}
	acCursor, err := h.DB.SocialAccounts().Find(ctx, accountFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load accounts"})
		return
	}
	var accounts []models.SocialAccount
	if err := acCursor.All(ctx, &accounts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode accounts"})
		return
	}

	// Build stats summary for the AI
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	totalPosts := len(posts)
	var published, failed, scheduled, drafts int
	var recentPosts, last7Posts int
	platformCounts := map[string]int{}
	typeCounts := map[string]int{}
	tagCounts := map[string]int{}
	hourCounts := map[int]int{}
	weekdayCounts := map[string]int{}
	var recentContents []string

	for _, p := range posts {
		switch p.Status {
		case models.PostStatusPublished:
			published++
		case models.PostStatusFailed:
			failed++
		case models.PostStatusScheduled:
			scheduled++
		case models.PostStatusDraft:
			drafts++
		}
		for _, pl := range p.Platforms {
			platformCounts[string(pl)]++
		}
		pt := string(p.PostType)
		if pt == "" {
			pt = "post"
		}
		typeCounts[pt]++
		for _, t := range p.Tags {
			tagCounts[t]++
		}
		if p.ScheduledAt.After(thirtyDaysAgo) {
			recentPosts++
			hourCounts[p.ScheduledAt.Hour()]++
			weekdayCounts[p.ScheduledAt.Weekday().String()]++
		}
		if p.ScheduledAt.After(sevenDaysAgo) {
			last7Posts++
		}
		// Collect recent content snippets (last 20 published posts)
		if p.Status == models.PostStatusPublished && len(recentContents) < 20 {
			snippet := p.Content
			if len(snippet) > 150 {
				snippet = snippet[:150] + "..."
			}
			if snippet != "" {
				recentContents = append(recentContents, snippet)
			}
		}
	}

	// Build account info
	accountInfo := []string{}
	for _, a := range accounts {
		accountInfo = append(accountInfo, fmt.Sprintf("%s (@%s)", a.Platform, a.AccountName))
	}

	// Build the data summary
	summary := fmt.Sprintf(`Social Media Dashboard Data:

Accounts: %s

Post Statistics (all time):
- Total posts: %d
- Published: %d
- Failed: %d
- Scheduled (upcoming): %d
- Drafts: %d

Last 30 days: %d posts
Last 7 days: %d posts

Platform distribution: %v
Post types: %v
Tags used: %v

Posting hours (last 30 days, 24h format): %v
Posting days (last 30 days): %v

Recent post content snippets:
%s`,
		fmt.Sprintf("%v", accountInfo),
		totalPosts, published, failed, scheduled, drafts,
		recentPosts, last7Posts,
		platformCounts, typeCounts, tagCounts,
		hourCounts, weekdayCounts,
		formatSnippets(recentContents),
	)

	model := settings.OpenRouterModel
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	langInstr := ""
	if settings.AILanguage != "" {
		langInstr = fmt.Sprintf(" Write all text values in %s.", settings.AILanguage)
	}
	systemPrompt := `You are a social media strategy analyst. Analyze the posting data provided and give actionable recommendations. Respond in valid JSON with this exact structure:
{
  "stats": [
    {"label": "short label", "value": "number or short text", "trend": "up|down|neutral"}
  ],
  "recommendations": [
    {"title": "short title", "description": "1-2 sentence actionable recommendation", "priority": "high|medium|low"}
  ]
}

Provide exactly 4 stats (pick the most insightful metrics like posting frequency, success rate, platform balance, consistency) and 3-5 recommendations based on the data. Recommendations should be specific and actionable (e.g. best times to post, content gaps, platform strategy, posting frequency suggestions). Reply ONLY with valid JSON, no markdown fences.` + langInstr

	reqBody, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": summary},
		},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+settings.OpenRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to reach OpenRouter: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("OpenRouter returned %d: %s", resp.StatusCode, string(body))})
		return
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Invalid response from OpenRouter"})
		return
	}

	// Parse the AI response as JSON and forward it
	aiText := result.Choices[0].Message.Content
	var insights json.RawMessage
	if err := json.Unmarshal([]byte(aiText), &insights); err != nil {
		// If the AI didn't return valid JSON, wrap it
		c.JSON(http.StatusOK, gin.H{
			"stats":           []any{},
			"recommendations": []any{},
			"raw":             aiText,
		})
		return
	}
	c.Data(http.StatusOK, "application/json", insights)
}

// ─── Team Admin Handlers ──────────────────────────────────────────────────────
// These operate on the caller's own team and are protected by TeamAdminRequired.

func (h *AdminHandler) TeamListAccounts(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	tid, err := primitive.ObjectIDFromHex(teamIDRaw.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.SocialAccounts().Find(ctx, bson.M{"teamId": tid})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch accounts"})
		return
	}
	defer cursor.Close(ctx)

	var accounts []models.SocialAccount
	if err := cursor.All(ctx, &accounts); err != nil {
		log.Printf("TeamListAccounts cursor.All error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode accounts"})
		return
	}
	if accounts == nil {
		accounts = []models.SocialAccount{}
	}
	c.JSON(http.StatusOK, accounts)
}

func (h *AdminHandler) TeamAddBlueskyAccount(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	tid, err := primitive.ObjectIDFromHex(teamIDRaw.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

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
		TeamID:      &tid,
		Platform:    models.PlatformBluesky,
		AccountName: input.Handle,
		DisplayName: input.Handle,
		AppPassword: input.AppPassword,
		PDSHost:     pdsHost,
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if h.Bluesky != nil {
		if dn, av, err := h.Bluesky.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
			}
			account.AvatarURL = av
		}
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

func (h *AdminHandler) TeamAddTwitterAccount(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	tid, err := primitive.ObjectIDFromHex(teamIDRaw.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var input AddTwitterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := models.SocialAccount{
		TeamID:            &tid,
		Platform:          models.PlatformTwitter,
		AccountName:       "twitter",
		DisplayName:       "Twitter Account",
		ConsumerKey:       input.ConsumerKey,
		ConsumerSecret:    input.ConsumerSecret,
		AccessToken:       input.AccessToken,
		AccessTokenSecret: input.AccessTokenSecret,
		IsActive:          true,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if h.Twitter != nil {
		if dn, username, av, err := h.Twitter.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
			}
			if username != "" {
				account.AccountName = username
			}
			account.AvatarURL = av
		}
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

func (h *AdminHandler) TeamAddMastodonAccount(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	tid, err := primitive.ObjectIDFromHex(teamIDRaw.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var input AddMastodonInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := models.SocialAccount{
		TeamID:           &tid,
		Platform:         models.PlatformMastodon,
		AccountName:      input.Instance,
		DisplayName:      input.Instance,
		AccessToken:      input.AccessToken,
		MastodonInstance: input.Instance,
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if h.Mastodon != nil {
		if dn, av, err := h.Mastodon.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
			}
			account.AvatarURL = av
		}
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

func (h *AdminHandler) TeamAddThreadsAccount(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	tid, err := primitive.ObjectIDFromHex(teamIDRaw.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var input AddThreadsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := models.SocialAccount{
		TeamID:        &tid,
		Platform:      models.PlatformThreads,
		AccountName:   "threads",
		DisplayName:   "Threads Account",
		AccessToken:   input.AccessToken,
		ThreadsUserID: input.ThreadsUserID,
		IsActive:      true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if h.Threads != nil {
		if dn, av, err := h.Threads.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
				account.AccountName = dn
			}
			account.AvatarURL = av
		}
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

func (h *AdminHandler) TeamAddLinkedInAccount(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	tid, err := primitive.ObjectIDFromHex(teamIDRaw.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var input AddLinkedInInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := models.SocialAccount{
		TeamID:            &tid,
		Platform:          models.PlatformLinkedIn,
		AccountName:       "linkedin",
		DisplayName:       "LinkedIn Account",
		AccessToken:       input.AccessToken,
		LinkedInPersonURN: input.LinkedInPersonURN,
		IsActive:          true,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if h.LinkedIn != nil {
		if dn, av, err := h.LinkedIn.FetchProfile(&account); err == nil {
			if dn != "" {
				account.DisplayName = dn
				account.AccountName = dn
			}
			account.AvatarURL = av
		}
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

func (h *AdminHandler) TeamDeleteAccount(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tid, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))
	result, err := h.DB.SocialAccounts().DeleteOne(ctx, bson.M{"_id": id, "teamId": tid})
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found or not in your team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Account deleted"})
}

func (h *AdminHandler) TeamToggleAccount(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tid, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))
	var account models.SocialAccount
	err = h.DB.SocialAccounts().FindOne(ctx, bson.M{"_id": id, "teamId": tid}).Decode(&account)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found or not in your team"})
		return
	}

	h.DB.SocialAccounts().UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"isActive": !account.IsActive, "updatedAt": time.Now()},
	})
	account.IsActive = !account.IsActive
	c.JSON(http.StatusOK, account)
}

func (h *AdminHandler) TeamInstagramAuthURL(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	state := "instagram_auth:" + teamIDRaw.(string)
	url, err := h.instagramAuthURL(ctx, state)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url})
}

func (h *AdminHandler) TeamListMembers(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	tid, err := primitive.ObjectIDFromHex(teamIDRaw.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.Users().Find(ctx, bson.M{"teamId": tid})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch members"})
		return
	}
	defer cursor.Close(ctx)

	var members []models.User
	if err := cursor.All(ctx, &members); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch members"})
		return
	}
	if members == nil {
		members = []models.User{}
	}
	c.JSON(http.StatusOK, members)
}

type TeamAddMemberInput struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *AdminHandler) TeamAddMember(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	tid, err := primitive.ObjectIDFromHex(teamIDRaw.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var input TeamAddMemberInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err = h.DB.Users().FindOne(ctx, bson.M{"email": input.Email}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No account found with that email address"})
		return
	}

	if user.TeamID != nil {
		if *user.TeamID == tid {
			c.JSON(http.StatusConflict, gin.H{"error": "User is already a member of this team"})
		} else {
			c.JSON(http.StatusConflict, gin.H{"error": "User is already a member of another team"})
		}
		return
	}

	_, err = h.DB.Users().UpdateOne(ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{"teamId": tid, "updatedAt": time.Now()}},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add member"})
		return
	}

	user.TeamID = &tid
	c.JSON(http.StatusOK, user)
}

func (h *AdminHandler) TeamRemoveMember(c *gin.Context) {
	teamIDRaw, _ := c.Get("teamId")
	callerIDRaw, _ := c.Get("userId")

	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if callerIDRaw.(string) == c.Param("id") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot remove yourself from the team"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tid, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))
	result, err := h.DB.Users().UpdateOne(ctx,
		bson.M{"_id": id, "teamId": tid},
		bson.M{
			"$unset": bson.M{"teamId": ""},
			"$set":   bson.M{"isTeamAdmin": false, "updatedAt": time.Now()},
		},
	)
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Member not found in your team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Member removed from team"})
}

func formatSnippets(snippets []string) string {
	if len(snippets) == 0 {
		return "(no published posts yet)"
	}
	result := ""
	for i, s := range snippets {
		result += fmt.Sprintf("%d. %s\n", i+1, s)
	}
	return result
}

// DashboardStats returns computed analytics derived entirely from the local
// database — no AI or external API calls required.
// GET /dashboard/stats
func (h *AdminHandler) DashboardStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- posts (scoped to team or user) ---
	cursor, err := h.DB.Posts().Find(ctx, postFilter(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load posts"})
		return
	}
	var posts []models.Post
	if err := cursor.All(ctx, &posts); err != nil {
		log.Printf("DashboardStats cursor.All error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode posts"})
		return
	}

	// --- inbox counts (scoped to team, or all for global admin) ---
	inboxScope := bson.M{}
	if teamIDRaw, hasTeam := c.Get("teamId"); hasTeam {
		tid, _ := primitive.ObjectIDFromHex(teamIDRaw.(string))
		inboxScope["teamId"] = tid
	} else {
		isAdminVal, _ := c.Get("isAdmin")
		if isAdminVal == nil || !isAdminVal.(bool) {
			// Non-admin without a team: no inbox access
			inboxScope["_id"] = primitive.NewObjectID()
		}
		// Global admin without a team: no extra filter → all messages
	}
	addInboxScope := func(f bson.M) bson.M {
		for k, v := range inboxScope {
			f[k] = v
		}
		return f
	}
	totalComments, _ := h.DB.InboxMessages().CountDocuments(ctx, addInboxScope(bson.M{"messageType": "comment"}))
	totalDMs, _ := h.DB.InboxMessages().CountDocuments(ctx, addInboxScope(bson.M{"messageType": "dm"}))
	unreadCount, _ := h.DB.InboxMessages().CountDocuments(ctx, addInboxScope(bson.M{"isRead": false}))
	likedCount, _ := h.DB.InboxMessages().CountDocuments(ctx, addInboxScope(bson.M{"isLiked": true, "messageType": "comment"}))

	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	// posts-by-day map (last 30 days)
	dayMap := map[string]int{}
	for i := 0; i < 30; i++ {
		d := now.AddDate(0, 0, -i)
		dayMap[d.Format("2006-01-02")] = 0
	}

	platformBreakdown := map[string]int{}
	typeBreakdown := map[string]int{}
	statusBreakdown := map[string]int{
		"published": 0, "failed": 0, "scheduled": 0, "draft": 0,
	}
	hourCounts := map[int]int{}
	weekdayAbbrs := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	weekdayCounts := map[string]int{}
	for _, d := range weekdayAbbrs {
		weekdayCounts[d] = 0
	}

	// week boundaries (Monday-based)
	todayMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	daysFromMon := int(now.Weekday()+6) % 7 // 0=Mon…6=Sun
	thisWeekStart := todayMidnight.AddDate(0, 0, -daysFromMon)
	lastWeekStart := thisWeekStart.AddDate(0, 0, -7)
	var thisWeekPosts, lastWeekPosts int
	var published, failed int

	for _, p := range posts {
		switch p.Status {
		case models.PostStatusPublished:
			statusBreakdown["published"]++
			published++
		case models.PostStatusFailed:
			statusBreakdown["failed"]++
			failed++
		case models.PostStatusScheduled:
			statusBreakdown["scheduled"]++
		case models.PostStatusDraft:
			statusBreakdown["draft"]++
		}

		for _, pl := range p.Platforms {
			platformBreakdown[string(pl)]++
		}

		pt := string(p.PostType)
		if pt == "" {
			pt = "post"
		}
		typeBreakdown[pt]++

		t := p.ScheduledAt
		if t.IsZero() {
			t = p.CreatedAt
		}

		if t.After(thirtyDaysAgo) {
			ds := t.Format("2006-01-02")
			if _, ok := dayMap[ds]; ok {
				dayMap[ds]++
			}
		}

		// Hour distribution — published posts only
		if p.Status == models.PostStatusPublished {
			hourCounts[t.Hour()]++
		}

		// Weekday (all statuses)
		wd := t.Weekday()
		abbr := wd.String()[:3] // "Mon", "Tue", etc.
		weekdayCounts[abbr]++

		// This / last week
		if !t.Before(thisWeekStart) {
			thisWeekPosts++
		} else if !t.Before(lastWeekStart) {
			lastWeekPosts++
		}
	}

	// Build sorted slice for postsByDay (oldest → newest)
	type DayCount struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	postsByDay := make([]DayCount, 30)
	for i := 0; i < 30; i++ {
		d := now.AddDate(0, 0, -(29 - i))
		ds := d.Format("2006-01-02")
		postsByDay[i] = DayCount{Date: ds, Count: dayMap[ds]}
	}

	// All 24 hours
	type HourCount struct {
		Hour  int `json:"hour"`
		Count int `json:"count"`
	}
	hourDistribution := make([]HourCount, 24)
	for i := 0; i < 24; i++ {
		hourDistribution[i] = HourCount{Hour: i, Count: hourCounts[i]}
	}

	// Mon–Sun
	type WeekdayCount struct {
		Day   string `json:"day"`
		Count int    `json:"count"`
	}
	weekdayDistribution := make([]WeekdayCount, 7)
	for i, d := range weekdayAbbrs {
		weekdayDistribution[i] = WeekdayCount{Day: d, Count: weekdayCounts[d]}
	}

	successRate := 0.0
	if published+failed > 0 {
		successRate = float64(published) / float64(published+failed) * 100
	}

	c.JSON(http.StatusOK, gin.H{
		"postsByDay":          postsByDay,
		"platformBreakdown":   platformBreakdown,
		"postTypeBreakdown":   typeBreakdown,
		"statusBreakdown":     statusBreakdown,
		"hourDistribution":    hourDistribution,
		"weekdayDistribution": weekdayDistribution,
		"thisWeekPosts":       thisWeekPosts,
		"lastWeekPosts":       lastWeekPosts,
		"successRate":         successRate,
		"inboxStats": gin.H{
			"comments": totalComments,
			"dms":      totalDMs,
			"unread":   unreadCount,
			"liked":    likedCount,
		},
	})
}

// Watermark gallery management

func watermarkFilter(c *gin.Context) bson.M {
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		return bson.M{"teamId": tid}
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	return bson.M{"userId": uid}
}

func (h *AdminHandler) ListWatermarks(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.Watermarks().Find(ctx, watermarkFilter(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch watermarks"})
		return
	}
	defer cursor.Close(ctx)

	var watermarks []models.Watermark
	if err := cursor.All(ctx, &watermarks); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode watermarks"})
		return
	}
	if watermarks == nil {
		watermarks = []models.Watermark{}
	}
	c.JSON(http.StatusOK, watermarks)
}

func (h *AdminHandler) UploadWatermark(c *gin.Context) {
	name := c.PostForm("name")
	if name == "" {
		name = "Watermark"
	}

	_, fh, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file provided"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	postHandler := &PostHandler{DB: h.DB, UploadDir: h.UploadDir}

	url, err := postHandler.saveUpload(ctx, fh)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	wm := models.Watermark{
		UserID:    objID,
		Name:      name,
		Filename:  fh.Filename,
		URL:       url,
		CreatedAt: time.Now(),
	}
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		wm.TeamID = &tid
	}

	result, err := h.DB.Watermarks().InsertOne(ctx, wm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save watermark"})
		return
	}
	wm.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, wm)
}

func (h *AdminHandler) DeleteWatermark(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := watermarkFilter(c)
	filter["_id"] = id
	result, err := h.DB.Watermarks().DeleteOne(ctx, filter)
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Watermark not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Watermark deleted"})
}
