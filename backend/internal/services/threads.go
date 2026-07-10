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

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const threadsGraphAPI = "https://graph.threads.net/v1.0"

type ThreadsService struct {
	DB *database.MongoDB
}

type threadsError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
}

func (e *threadsError) Error() string {
	return fmt.Sprintf("Threads error (code=%d, type=%s): %s", e.Code, e.Type, e.Message)
}

func (s *ThreadsService) Post(ctx context.Context, content string, imageURLs []string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active Threads account: %w", err)
	}

	var settings models.AppSettings
	s.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	baseURL := settings.AppURL

	var containerID string
	switch {
	case len(imageURLs) == 1:
		imageURL := imageURLs[0]
		if !strings.HasPrefix(imageURL, "http") {
			imageURL = baseURL + imageURL
		}
		containerID, err = s.createContainer(account, content, "IMAGE", imageURL, nil)
	case len(imageURLs) > 1:
		containerID, err = s.createCarousel(account, content, imageURLs, baseURL)
	default:
		containerID, err = s.createContainer(account, content, "TEXT", "", nil)
	}
	if err != nil {
		return "", err
	}

	return s.publishContainer(account, containerID)
}

func (s *ThreadsService) createContainer(account *models.SocialAccount, text, mediaType, imageURL string, childIDs []string) (string, error) {
	params := url.Values{
		"media_type":   {mediaType},
		"access_token": {account.AccessToken},
	}
	if text != "" {
		params.Set("text", text)
	}
	if imageURL != "" {
		params.Set("image_url", imageURL)
	}
	if len(childIDs) > 0 {
		params.Set("children", strings.Join(childIDs, ","))
	}

	resp, err := http.PostForm(fmt.Sprintf("%s/%s/threads", threadsGraphAPI, account.ThreadsUserID), params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Threads create container: status=%d body=%s", resp.StatusCode, string(body))

	var result struct {
		ID    string        `json:"id"`
		Error *threadsError `json:"error"`
	}
	json.Unmarshal(body, &result)
	if result.Error != nil {
		return "", result.Error
	}
	return result.ID, nil
}

func (s *ThreadsService) createCarousel(account *models.SocialAccount, text string, imageURLs []string, baseURL string) (string, error) {
	var childIDs []string
	for _, imgURL := range imageURLs {
		if len(childIDs) >= 10 {
			break
		}
		fullURL := imgURL
		if !strings.HasPrefix(imgURL, "http") {
			fullURL = baseURL + imgURL
		}
		childID, err := s.createContainer(account, "", "IMAGE", fullURL, nil)
		if err != nil {
			log.Printf("Threads carousel child error: %v", err)
			continue
		}
		childIDs = append(childIDs, childID)
	}
	if len(childIDs) == 0 {
		return "", fmt.Errorf("no carousel items created")
	}
	return s.createContainer(account, text, "CAROUSEL", "", childIDs)
}

func (s *ThreadsService) publishContainer(account *models.SocialAccount, containerID string) (string, error) {
	params := url.Values{
		"creation_id":  {containerID},
		"access_token": {account.AccessToken},
	}

	resp, err := http.PostForm(fmt.Sprintf("%s/%s/threads_publish", threadsGraphAPI, account.ThreadsUserID), params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Threads publish: status=%d body=%s", resp.StatusCode, string(body))

	var result struct {
		ID    string        `json:"id"`
		Error *threadsError `json:"error"`
	}
	json.Unmarshal(body, &result)
	if result.Error != nil {
		return "", result.Error
	}
	return result.ID, nil
}

// FetchProfile fetches the Threads user's display name and avatar, and sets
// account.ThreadsUserID as a side effect if the API returns a user ID.
func (s *ThreadsService) FetchProfile(account *models.SocialAccount) (displayName, avatarURL string, err error) {
	resp, err := http.Get(fmt.Sprintf(
		"%s/me?fields=id,username,threads_profile_picture_url&access_token=%s",
		threadsGraphAPI, account.AccessToken))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var profile struct {
		ID                       string        `json:"id"`
		Username                 string        `json:"username"`
		ThreadsProfilePictureURL string        `json:"threads_profile_picture_url"`
		Error                    *threadsError `json:"error"`
	}
	json.Unmarshal(body, &profile)
	if profile.Error != nil {
		return "", "", profile.Error
	}

	if profile.ID != "" {
		account.ThreadsUserID = profile.ID
	}
	return profile.Username, profile.ThreadsProfilePictureURL, nil
}

func (s *ThreadsService) getAccount(ctx context.Context, accountID string) (*models.SocialAccount, error) {
	if accountID == "" {
		return nil, fmt.Errorf("threads account ID is required")
	}
	id, err := primitive.ObjectIDFromHex(accountID)
	if err != nil {
		return nil, fmt.Errorf("invalid threads account ID: %s", accountID)
	}
	var account models.SocialAccount
	err = s.DB.SocialAccounts().FindOne(ctx, bson.M{
		"platform": models.PlatformThreads,
		"isActive": true,
		"_id":      id,
	}).Decode(&account)
	return &account, err
}
