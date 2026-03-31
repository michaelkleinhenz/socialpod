package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	// Video containers can take several minutes to process on Instagram's side.
	// Poll for up to 5 minutes with increasing intervals.
	maxAttempts := 60
	for i := 0; i < maxAttempts; i++ {
		resp, err := http.Get(fmt.Sprintf(
			"%s/%s?fields=status_code,status&access_token=%s",
			igGraphAPI, containerID, account.AccessToken))
		if err != nil {
			return err
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var status struct {
			StatusCode string `json:"status_code"`
			Status     string `json:"status"`
			Error      *struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		json.Unmarshal(body, &status)

		if i == 0 || i%10 == 0 || status.StatusCode == "FINISHED" || status.StatusCode == "ERROR" || status.Error != nil {
			log.Printf("IG container %s: status_code=%q status=%q raw=%s (attempt %d/%d)",
				containerID, status.StatusCode, status.Status, string(body), i+1, maxAttempts)
		}

		if status.Error != nil {
			return fmt.Errorf("IG container check error (code %d): %s", status.Error.Code, status.Error.Message)
		}

		switch status.StatusCode {
		case "FINISHED":
			return nil
		case "ERROR":
			return fmt.Errorf("container processing failed: %s", status.Status)
		}

		// Wait 3s for the first 10 polls, then 5s after that.
		if i < 10 {
			time.Sleep(3 * time.Second)
		} else {
			time.Sleep(5 * time.Second)
		}
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

// PostStory publishes an Instagram Story. mediaURL must be a publicly
// reachable URL pointing to an image or video file.
func (s *InstagramService) PostStory(ctx context.Context, mediaURL string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active Instagram account: %w", err)
	}

	var settings models.AppSettings
	s.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)

	fullURL := mediaURL
	if !strings.HasPrefix(mediaURL, "http") {
		fullURL = settings.AppURL + mediaURL
	}

	params := url.Values{
		"media_type":   {"STORIES"},
		"access_token": {account.AccessToken},
	}

	// Determine if the media is a video based on extension.
	lowerURL := strings.ToLower(fullURL)
	if strings.HasSuffix(lowerURL, ".mp4") || strings.HasSuffix(lowerURL, ".mov") {
		params.Set("video_url", fullURL)
	} else {
		params.Set("image_url", fullURL)
	}

	log.Printf("IG story create: url=%s params=%v", fullURL, params)
	resp, err := http.PostForm(fmt.Sprintf("%s/%s/media", igGraphAPI, account.IGUserID), params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("IG story container response: %s", string(body))

	var container struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(body, &container)
	if container.Error != nil {
		return "", fmt.Errorf("IG story error: %s", container.Error.Message)
	}

	return s.publishContainer(account, container.ID)
}

// PostReel publishes an Instagram Reel. mediaURL must be a publicly reachable
// URL pointing to an MP4 video file.
func (s *InstagramService) PostReel(ctx context.Context, mediaURL string, caption string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active Instagram account: %w", err)
	}

	var settings models.AppSettings
	s.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)

	fullURL := mediaURL
	if !strings.HasPrefix(mediaURL, "http") {
		fullURL = settings.AppURL + mediaURL
	}

	params := url.Values{
		"media_type":   {"REELS"},
		"video_url":    {fullURL},
		"caption":      {caption},
		"access_token": {account.AccessToken},
	}

	log.Printf("IG reel create: url=%s", fullURL)
	resp, err := http.PostForm(fmt.Sprintf("%s/%s/media", igGraphAPI, account.IGUserID), params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("IG reel container response: %s", string(body))

	var container struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(body, &container)
	if container.Error != nil {
		return "", fmt.Errorf("IG reel error: %s", container.Error.Message)
	}

	return s.publishContainer(account, container.ID)
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

// ReplyToComment posts a reply to an existing Instagram comment.
func (s *InstagramService) ReplyToComment(ctx context.Context, commentID, text, accountID string) error {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("no active Instagram account: %w", err)
	}

	params := url.Values{
		"message":      {text},
		"access_token": {account.AccessToken},
	}

	resp, err := http.PostForm(fmt.Sprintf("%s/%s/replies", igGraphAPI, commentID), params)
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
		return fmt.Errorf("IG reply error: %s", result.Error.Message)
	}
	return nil
}

// SendDM sends a direct message to an Instagram user.
// recipientID is the Instagram-Scoped User ID (IGSID) from the webhook.
func (s *InstagramService) SendDM(ctx context.Context, recipientID, text, accountID string) error {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("no active Instagram account: %w", err)
	}

	body := map[string]interface{}{
		"recipient":    map[string]string{"id": recipientID},
		"message":      map[string]string{"text": text},
		"access_token": account.AccessToken,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/%s/messages", igGraphAPI, account.IGUserID),
		"application/json",
		strings.NewReader(string(bodyJSON)),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		MessageID string `json:"message_id"`
		Error     *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error != nil {
		return fmt.Errorf("IG DM error: %s", result.Error.Message)
	}
	return nil
}

// IGMedia represents a single media item from the user's Instagram feed.
type IGMedia struct {
	ID            string `json:"id"`
	Caption       string `json:"caption,omitempty"`
	MediaType     string `json:"mediaType"`
	MediaURL      string `json:"mediaUrl,omitempty"`
	ThumbnailURL  string `json:"thumbnailUrl,omitempty"`
	Permalink     string `json:"permalink"`
	Timestamp     string `json:"timestamp"`
	LikeCount     int    `json:"likeCount"`
	CommentsCount int    `json:"commentsCount"`
}

// FetchFeed retrieves the authenticated user's own Instagram media feed.
func (s *InstagramService) FetchFeed(ctx context.Context, accountID string) ([]IGMedia, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("no active Instagram account: %w", err)
	}

	resp, err := http.Get(fmt.Sprintf(
		"%s/%s/media?fields=id,caption,media_type,media_url,thumbnail_url,permalink,timestamp,like_count,comments_count&access_token=%s",
		igGraphAPI, account.IGUserID, account.AccessToken))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID            string `json:"id"`
			Caption       string `json:"caption"`
			MediaType     string `json:"media_type"`
			MediaURL      string `json:"media_url"`
			ThumbnailURL  string `json:"thumbnail_url"`
			Permalink     string `json:"permalink"`
			Timestamp     string `json:"timestamp"`
			LikeCount     int    `json:"like_count"`
			CommentsCount int    `json:"comments_count"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error != nil {
		return nil, fmt.Errorf("IG feed error: %s", result.Error.Message)
	}

	media := make([]IGMedia, 0, len(result.Data))
	for _, item := range result.Data {
		media = append(media, IGMedia{
			ID:            item.ID,
			Caption:       item.Caption,
			MediaType:     item.MediaType,
			MediaURL:      item.MediaURL,
			ThumbnailURL:  item.ThumbnailURL,
			Permalink:     item.Permalink,
			Timestamp:     item.Timestamp,
			LikeCount:     item.LikeCount,
			CommentsCount: item.CommentsCount,
		})
	}
	return media, nil
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
		"%s/me?fields=user_id,username,profile_picture_url&access_token=%s",
		igGraphAPI, token))
	if err != nil {
		return nil, err
	}
	defer profileResp.Body.Close()

	var profile struct {
		ID                string `json:"id"`
		Username          string `json:"username"`
		ProfilePictureURL string `json:"profile_picture_url"`
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
		AvatarURL:   profile.ProfilePictureURL,
		IsActive:    true,
	}

	return account, nil
}
