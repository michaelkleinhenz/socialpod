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
	cursor.All(ctx, &accounts)
	if accounts == nil {
		accounts = []models.SocialAccount{}
	}

	c.JSON(http.StatusOK, accounts)
}

// ListActiveAccounts returns active accounts for all authenticated users (no secrets exposed).
func (h *AdminHandler) ListActiveAccounts(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.SocialAccounts().Find(ctx, bson.M{"isActive": true})
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
	}
	c.JSON(http.StatusOK, settingsResponse{
		AppSettings:     settings,
		HasOpenRouterKey: settings.OpenRouterAPIKey != "",
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
		"&scope=instagram_business_basic,instagram_business_content_publish,instagram_business_manage_messages,instagram_business_manage_comments" +
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
	}
	if accountFound {
		msg.AccountID = account.ID
		msg.AccountName = account.AccountName
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

	// Skip echo
	if accountFound && messaging.Sender.ID == account.IGUserID {
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
	cursor.All(ctx, &users)
	if users == nil {
		users = []models.User{}
	}

	c.JSON(http.StatusOK, users)
}

type CreateUserInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Name     string `json:"name" binding:"required"`
	IsAdmin  bool   `json:"isAdmin"`
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
		Email:     input.Email,
		Password:  string(hash),
		Name:      input.Name,
		IsAdmin:   input.IsAdmin,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result, err := h.DB.Users().InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	user.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, user)
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
	cursor.All(ctx, &teams)
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

func (h *AdminHandler) DeleteTeam(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Remove team assignment from all members
	h.DB.Users().UpdateMany(ctx, bson.M{"teamId": id}, bson.M{"$unset": bson.M{"teamId": ""}})

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

	// Remove all current members from this team
	h.DB.Users().UpdateMany(ctx, bson.M{"teamId": teamID}, bson.M{"$unset": bson.M{"teamId": ""}})

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
	for _, p := range input.Platforms {
		if p == "bluesky" {
			systemPrompt += " The post must fit within 300 characters."
			break
		}
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

// Watermark gallery management

func (h *AdminHandler) ListWatermarks(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.Watermarks().Find(ctx, bson.M{})
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

	wm := models.Watermark{
		Name:      name,
		Filename:  fh.Filename,
		URL:       url,
		CreatedAt: time.Now(),
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

	result, err := h.DB.Watermarks().DeleteOne(ctx, bson.M{"_id": id})
	if err != nil || result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Watermark not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Watermark deleted"})
}
