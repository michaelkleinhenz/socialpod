package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	youtubeTokenURL   = "https://oauth2.googleapis.com/token"
	youtubeUploadURL  = "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=multipart&part=snippet,status"
	youtubeChannelURL = "https://www.googleapis.com/youtube/v3/channels?part=snippet&mine=true"
)

type YouTubeService struct {
	DB        *database.MongoDB
	UploadDir string
}

func (s *YouTubeService) Post(ctx context.Context, content string, imageURLs []string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active YouTube account: %w", err)
	}

	if len(imageURLs) == 0 {
		return "", fmt.Errorf("YouTube post requires at least one image")
	}

	var settings models.AppSettings
	s.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)

	imgURL := imageURLs[0]
	if !strings.HasPrefix(imgURL, "http") {
		imgURL = settings.AppURL + imgURL
	}

	accessToken, err := s.ensureValidToken(ctx, account, &settings)
	if err != nil {
		return "", fmt.Errorf("failed to refresh YouTube token: %w", err)
	}

	return s.uploadVideo(ctx, accessToken, content, imgURL, false)
}

func (s *YouTubeService) PostShort(ctx context.Context, videoURL string, caption string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active YouTube account: %w", err)
	}

	var settings models.AppSettings
	s.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)

	fullURL := videoURL
	if !strings.HasPrefix(videoURL, "http") {
		fullURL = settings.AppURL + videoURL
	}

	accessToken, err := s.ensureValidToken(ctx, account, &settings)
	if err != nil {
		return "", fmt.Errorf("failed to refresh YouTube token: %w", err)
	}

	return s.uploadVideo(ctx, accessToken, caption, fullURL, true)
}

func (s *YouTubeService) uploadVideo(ctx context.Context, accessToken, description, mediaURL string, isShort bool) (string, error) {
	mediaData, contentType, err := s.fetchMedia(mediaURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch media: %w", err)
	}

	title := description
	if len(title) > 100 {
		title = title[:97] + "..."
	}
	if title == "" {
		title = "Post"
	}

	snippet := map[string]interface{}{
		"snippet": map[string]interface{}{
			"title":       title,
			"description": description,
			"categoryId":  "22",
		},
		"status": map[string]interface{}{
			"privacyStatus":           "public",
			"selfDeclaredMadeForKids": false,
		},
	}

	if isShort {
		snippet["snippet"].(map[string]interface{})["title"] = "#Shorts " + title
	}

	metaJSON, _ := json.Marshal(snippet)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metaPart, err := writer.CreatePart(metaHeader)
	if err != nil {
		return "", err
	}
	metaPart.Write(metaJSON)

	mediaHeader := make(textproto.MIMEHeader)
	mediaHeader.Set("Content-Type", contentType)
	mediaPart, err := writer.CreatePart(mediaHeader)
	if err != nil {
		return "", err
	}
	mediaPart.Write(mediaData)

	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", youtubeUploadURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("YouTube upload response: status=%d body=%s", resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("YouTube upload failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	json.Unmarshal(respBody, &result)
	if result.ID == "" {
		return "", fmt.Errorf("YouTube upload returned no video ID")
	}

	return result.ID, nil
}

func (s *YouTubeService) fetchMedia(mediaURL string) ([]byte, string, error) {
	if strings.HasPrefix(mediaURL, "http://") || strings.HasPrefix(mediaURL, "https://") {
		resp, err := http.Get(mediaURL)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", err
		}
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "video/mp4"
		}
		return data, ct, nil
	}

	path := filepath.Join(s.UploadDir, filepath.Base(mediaURL))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	ext := strings.ToLower(filepath.Ext(path))
	ct := "video/mp4"
	if ext == ".mov" {
		ct = "video/quicktime"
	}
	return data, ct, nil
}

func (s *YouTubeService) ensureValidToken(ctx context.Context, account *models.SocialAccount, settings *models.AppSettings) (string, error) {
	if account.TokenExpiry.IsZero() || time.Now().Before(account.TokenExpiry.Add(-5*time.Minute)) {
		return account.AccessToken, nil
	}

	if account.RefreshToken == "" {
		return account.AccessToken, nil
	}

	params := url.Values{
		"client_id":     {settings.YouTubeClientID},
		"client_secret": {settings.YouTubeClientSecret},
		"refresh_token": {account.RefreshToken},
		"grant_type":    {"refresh_token"},
	}

	resp, err := http.PostForm(youtubeTokenURL, params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)

	if tokenResp.AccessToken == "" {
		return account.AccessToken, nil
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	s.DB.SocialAccounts().UpdateOne(ctx, bson.M{"_id": account.ID}, bson.M{
		"$set": bson.M{
			"accessToken": tokenResp.AccessToken,
			"tokenExpiry": expiry,
			"updatedAt":   time.Now(),
		},
	})

	return tokenResp.AccessToken, nil
}

func (s *YouTubeService) ExchangeCodeForToken(ctx context.Context, code, clientID, clientSecret, redirectURI string) (*models.SocialAccount, error) {
	params := url.Values{
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	resp, err := http.PostForm(youtubeTokenURL, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("YouTube token exchange response: status=%d body=%s", resp.StatusCode, string(body))

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	json.Unmarshal(body, &tokenResp)

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("failed to get YouTube access token")
	}

	channelName, channelID, avatarURL := s.fetchChannelInfo(tokenResp.AccessToken)

	account := &models.SocialAccount{
		Platform:         models.PlatformYouTube,
		AccountName:      channelName,
		DisplayName:      channelName,
		AccessToken:      tokenResp.AccessToken,
		RefreshToken:     tokenResp.RefreshToken,
		TokenExpiry:      time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		YouTubeChannelID: channelID,
		AvatarURL:        avatarURL,
		IsActive:         true,
	}

	return account, nil
}

func (s *YouTubeService) fetchChannelInfo(accessToken string) (name, channelID, avatarURL string) {
	req, _ := http.NewRequest("GET", youtubeChannelURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "YouTube Channel", "", ""
	}
	defer resp.Body.Close()

	var result struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title      string `json:"title"`
				Thumbnails struct {
					Default struct {
						URL string `json:"url"`
					} `json:"default"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Items) > 0 {
		item := result.Items[0]
		return item.Snippet.Title, item.ID, item.Snippet.Thumbnails.Default.URL
	}
	return "YouTube Channel", "", ""
}

func (s *YouTubeService) FetchProfile(account *models.SocialAccount) (string, string, error) {
	name, _, avatar := s.fetchChannelInfo(account.AccessToken)
	return name, avatar, nil
}

func (s *YouTubeService) getAccount(ctx context.Context, accountID string) (*models.SocialAccount, error) {
	if accountID == "" {
		return nil, fmt.Errorf("YouTube account ID is required")
	}
	id, err := primitive.ObjectIDFromHex(accountID)
	if err != nil || id == primitive.NilObjectID {
		return nil, fmt.Errorf("invalid YouTube account ID: %s", accountID)
	}
	var account models.SocialAccount
	if err := s.DB.SocialAccounts().FindOne(ctx, bson.M{
		"platform": models.PlatformYouTube,
		"isActive": true,
		"_id":      id,
	}).Decode(&account); err != nil {
		return nil, fmt.Errorf("YouTube account not found: %w", err)
	}
	return &account, nil
}
