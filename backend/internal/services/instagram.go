package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type InstagramService struct {
	DB *database.MongoDB
}

const igGraphAPI = "https://graph.instagram.com/v21.0"

func (s *InstagramService) Post(ctx context.Context, content string, imageURLs []string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active Instagram account: %w", err)
	}

	if len(imageURLs) == 0 {
		return "", fmt.Errorf("Instagram requires at least one image")
	}

	// Get app settings for the public URL
	var settings models.AppSettings
	s.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	baseURL := settings.AppURL

	if len(imageURLs) == 1 {
		return s.postSingleImage(ctx, account, content, baseURL+imageURLs[0])
	}
	return s.postCarousel(ctx, account, content, imageURLs, baseURL)
}

func (s *InstagramService) postSingleImage(ctx context.Context, account *models.SocialAccount, caption, imageURL string) (string, error) {
	// Step 1: Create media container
	params := url.Values{
		"image_url":    {imageURL},
		"caption":      {caption},
		"access_token": {account.AccessToken},
	}

	resp, err := http.PostForm(fmt.Sprintf("%s/%s/media", igGraphAPI, account.IGUserID), params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var container struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&container)
	if container.Error != nil {
		return "", fmt.Errorf("IG error: %s", container.Error.Message)
	}

	// Step 2: Publish
	return s.publishContainer(account, container.ID)
}

func (s *InstagramService) postCarousel(ctx context.Context, account *models.SocialAccount, caption string, imageURLs []string, baseURL string) (string, error) {
	var childIDs []string

	// Create child containers
	for _, imgURL := range imageURLs {
		if len(childIDs) >= 10 { // IG carousel max
			break
		}
		fullURL := imgURL
		if !strings.HasPrefix(imgURL, "http") {
			fullURL = baseURL + imgURL
		}

		params := url.Values{
			"image_url":      {fullURL},
			"is_carousel_item": {"true"},
			"access_token":   {account.AccessToken},
		}

		resp, err := http.PostForm(fmt.Sprintf("%s/%s/media", igGraphAPI, account.IGUserID), params)
		if err != nil {
			continue
		}

		var container struct {
			ID string `json:"id"`
		}
		json.NewDecoder(resp.Body).Decode(&container)
		resp.Body.Close()

		if container.ID != "" {
			childIDs = append(childIDs, container.ID)
		}
	}

	if len(childIDs) == 0 {
		return "", fmt.Errorf("no carousel items created")
	}

	// Create carousel container
	params := url.Values{
		"media_type":   {"CAROUSEL"},
		"caption":      {caption},
		"children":     {strings.Join(childIDs, ",")},
		"access_token": {account.AccessToken},
	}

	resp, err := http.PostForm(fmt.Sprintf("%s/%s/media", igGraphAPI, account.IGUserID), params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var container struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&container)

	return s.publishContainer(account, container.ID)
}

func (s *InstagramService) waitForContainer(account *models.SocialAccount, containerID string) error {
	for i := 0; i < 30; i++ {
		resp, err := http.Get(fmt.Sprintf(
			"%s/%s?fields=status_code&access_token=%s",
			igGraphAPI, containerID, account.AccessToken))
		if err != nil {
			return err
		}

		var status struct {
			StatusCode string `json:"status_code"`
		}
		json.NewDecoder(resp.Body).Decode(&status)
		resp.Body.Close()

		switch status.StatusCode {
		case "FINISHED":
			return nil
		case "ERROR":
			return fmt.Errorf("container processing failed")
		}

		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("container processing timed out")
}

func (s *InstagramService) publishContainer(account *models.SocialAccount, containerID string) (string, error) {
	if err := s.waitForContainer(account, containerID); err != nil {
		return "", err
	}

	params := url.Values{
		"creation_id":  {containerID},
		"access_token": {account.AccessToken},
	}

	resp, err := http.PostForm(fmt.Sprintf("%s/%s/media_publish", igGraphAPI, account.IGUserID), params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error != nil {
		return "", fmt.Errorf("publish error: %s", result.Error.Message)
	}

	return result.ID, nil
}

// PostComment posts a comment on an existing Instagram media object.
func (s *InstagramService) PostComment(ctx context.Context, text, mediaID, accountID string) error {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("no active Instagram account: %w", err)
	}

	params := url.Values{
		"message":      {text},
		"access_token": {account.AccessToken},
	}

	resp, err := http.PostForm(fmt.Sprintf("%s/%s/comments", igGraphAPI, mediaID), params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error != nil {
		return fmt.Errorf("IG comment error: %s", result.Error.Message)
	}
	return nil
}

func (s *InstagramService) getAccount(ctx context.Context, accountID string) (*models.SocialAccount, error) {
	filter := bson.M{"platform": models.PlatformInstagram, "isActive": true}
	if accountID != "" {
		if id, err := primitive.ObjectIDFromHex(accountID); err == nil {
			filter["_id"] = id
		}
	}

	var account models.SocialAccount
	err := s.DB.SocialAccounts().FindOne(ctx, filter).Decode(&account)
	return &account, err
}

// ExchangeCodeForToken handles the Instagram OAuth code exchange
func (s *InstagramService) ExchangeCodeForToken(ctx context.Context, code, clientID, clientSecret, redirectURI string) (*models.SocialAccount, error) {
	// Exchange code for short-lived token
	params := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
		"code":          {code},
	}

	resp, err := http.PostForm("https://api.instagram.com/oauth/access_token", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var shortToken struct {
		AccessToken string `json:"access_token"`
		UserID      int64  `json:"user_id"`
	}
	json.NewDecoder(resp.Body).Decode(&shortToken)

	if shortToken.AccessToken == "" {
		return nil, fmt.Errorf("failed to get access token")
	}

	// Exchange for long-lived token
	llResp, err := http.Get(fmt.Sprintf(
		"%s/access_token?grant_type=ig_exchange_token&client_secret=%s&access_token=%s",
		igGraphAPI, clientSecret, shortToken.AccessToken))
	if err != nil {
		return nil, err
	}
	defer llResp.Body.Close()

	var longToken struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	json.NewDecoder(llResp.Body).Decode(&longToken)

	token := longToken.AccessToken
	if token == "" {
		token = shortToken.AccessToken
	}

	// Get user profile
	profileResp, err := http.Get(fmt.Sprintf(
		"%s/me?fields=user_id,username&access_token=%s",
		igGraphAPI, token))
	if err != nil {
		return nil, err
	}
	defer profileResp.Body.Close()

	var profile struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	json.NewDecoder(profileResp.Body).Decode(&profile)

	igUserID := profile.ID
	if igUserID == "" {
		igUserID = fmt.Sprintf("%d", shortToken.UserID)
	}

	account := &models.SocialAccount{
		Platform:    models.PlatformInstagram,
		AccountName: profile.Username,
		DisplayName: profile.Username,
		AccessToken: token,
		IGUserID:    igUserID,
		IsActive:    true,
	}

	return account, nil
}
