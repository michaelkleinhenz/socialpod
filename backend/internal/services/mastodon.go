package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MastodonService struct {
	DB        *database.MongoDB
	UploadDir string
}

func (s *MastodonService) getAccount(ctx context.Context, accountID string) (*models.SocialAccount, error) {
	if accountID == "" {
		return nil, fmt.Errorf("mastodon account ID is required")
	}
	id, err := primitive.ObjectIDFromHex(accountID)
	if err != nil {
		return nil, fmt.Errorf("invalid mastodon account ID: %s", accountID)
	}

	var account models.SocialAccount
	err = s.DB.SocialAccounts().FindOne(ctx, bson.M{
		"platform": models.PlatformMastodon,
		"isActive": true,
		"_id":      id,
	}).Decode(&account)
	return &account, err
}

func (s *MastodonService) apiURL(account *models.SocialAccount) string {
	instance := strings.TrimRight(account.MastodonInstance, "/")
	if !strings.HasPrefix(instance, "http") {
		instance = "https://" + instance
	}
	return instance
}

// FetchProfile returns the display name and avatar URL for the given account credentials.
func (s *MastodonService) FetchProfile(account *models.SocialAccount) (displayName, avatarURL string, err error) {
	req, err := http.NewRequest("GET", s.apiURL(account)+"/api/v1/accounts/verify_credentials", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("verify_credentials failed (%d): %s", resp.StatusCode, string(body))
	}

	var profile struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Avatar      string `json:"avatar"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return "", "", err
	}

	name := profile.DisplayName
	if name == "" {
		name = profile.Username
	}
	return name, profile.Avatar, nil
}

// Post creates a new status on Mastodon, uploading any images first.
func (s *MastodonService) Post(ctx context.Context, content string, imageURLs []string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active Mastodon account: %w", err)
	}

	var mediaIDs []string
	for _, url := range imageURLs {
		if len(mediaIDs) >= 4 {
			break
		}
		id, uploadErr := s.uploadMedia(ctx, account, url)
		if uploadErr != nil {
			continue
		}
		mediaIDs = append(mediaIDs, id)
	}

	payload := map[string]interface{}{
		"status": content,
	}
	if len(mediaIDs) > 0 {
		payload["media_ids"] = mediaIDs
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", s.apiURL(account)+"/api/v1/statuses", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("post failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.URL != "" {
		return result.URL, nil
	}
	return result.ID, nil
}

func (s *MastodonService) uploadMedia(ctx context.Context, account *models.SocialAccount, imageURL string) (string, error) {
	filename := strings.TrimPrefix(imageURL, "/api/uploads/")
	var fileData []byte
	diskPath := filepath.Join(s.UploadDir, filename)
	if d, readErr := os.ReadFile(diskPath); readErr == nil {
		fileData = d
	} else {
		var upload models.Upload
		err := s.DB.Uploads().FindOne(ctx, bson.M{"filename": filename}).Decode(&upload)
		if err != nil {
			return "", fmt.Errorf("upload not found: %s", filename)
		}
		fileData = upload.Data
	}

	data, contentType := ResizeImageIfNeeded(fileData, filename)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	ext := "jpg"
	if strings.HasSuffix(strings.ToLower(filename), ".png") {
		ext = "png"
	} else if strings.HasSuffix(strings.ToLower(filename), ".gif") {
		ext = "gif"
	} else if strings.HasSuffix(strings.ToLower(filename), ".mp4") {
		ext = "mp4"
	}

	part, err := mw.CreateFormFile("file", "upload."+ext)
	if err != nil {
		return "", err
	}
	part.Write(data)
	_ = contentType
	mw.Close()

	uploadCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(uploadCtx, "POST", s.apiURL(account)+"/api/v2/media", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 202 means async processing; 200/201 means ready
	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 202 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("media upload failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode == 202 {
		// Poll until the media is processed (up to 30s)
		result.ID, err = s.waitForMedia(uploadCtx, account, result.ID)
		if err != nil {
			return result.ID, nil // return ID anyway; server may accept it
		}
	}

	return result.ID, nil
}

func (s *MastodonService) waitForMedia(ctx context.Context, account *models.SocialAccount, mediaID string) (string, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return mediaID, ctx.Err()
		default:
		}
		time.Sleep(2 * time.Second)

		req, _ := http.NewRequestWithContext(ctx, "GET",
			s.apiURL(account)+"/api/v1/media/"+mediaID, nil)
		req.Header.Set("Authorization", "Bearer "+account.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return mediaID, nil
		}
	}
	return mediaID, nil
}
