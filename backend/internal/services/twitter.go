package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TwitterService struct {
	DB        *database.MongoDB
	UploadDir string
}

const (
	twitterAPIBase    = "https://api.twitter.com/2"
	twitterUploadBase = "https://upload.twitter.com/1"
)

func (s *TwitterService) Post(ctx context.Context, content string, imageURLs []string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active Twitter account: %w", err)
	}

	var mediaIDs []string
	for i, imgURL := range imageURLs {
		if i >= 4 {
			break
		}
		mediaID, uploadErr := s.uploadMedia(ctx, account, imgURL)
		if uploadErr != nil {
			continue
		}
		mediaIDs = append(mediaIDs, mediaID)
	}

	tweetBody := map[string]interface{}{
		"text": content,
	}
	if len(mediaIDs) > 0 {
		tweetBody["media"] = map[string]interface{}{
			"media_ids": mediaIDs,
		}
	}

	jsonBody, _ := json.Marshal(tweetBody)
	endpoint := twitterAPIBase + "/tweets"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", s.buildOAuthHeader("POST", endpoint, nil, account))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		if resp.StatusCode == http.StatusForbidden && strings.Contains(string(body), "oauth1-permissions") {
			return "", fmt.Errorf("twitter post failed: app permissions are set to Read-only. " +
				"Go to the X Developer Portal → your app → Settings → User authentication settings, " +
				"change App permissions to 'Read and Write', then regenerate your Access Token and Secret")
		}
		return "", fmt.Errorf("twitter post failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)
	return result.Data.ID, nil
}

// FetchProfile retrieves the display name, username, and avatar for a Twitter account.
func (s *TwitterService) FetchProfile(account *models.SocialAccount) (displayName, username, avatarURL string, err error) {
	endpoint := twitterAPIBase + "/users/me"
	params := url.Values{"user.fields": {"name,username,profile_image_url"}}

	req, err := http.NewRequest("GET", endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Authorization", s.buildOAuthHeader("GET", endpoint, params, account))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("twitter profile fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Username        string `json:"username"`
			ProfileImageURL string `json:"profile_image_url"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)

	account.TwitterUserID = result.Data.ID
	return result.Data.Name, result.Data.Username, result.Data.ProfileImageURL, nil
}

func (s *TwitterService) uploadMedia(ctx context.Context, account *models.SocialAccount, imageURL string) (string, error) {
	filename := strings.TrimPrefix(imageURL, "/api/uploads/")
	var fileData []byte

	diskPath := filepath.Join(s.UploadDir, filename)
	if d, readErr := os.ReadFile(diskPath); readErr == nil {
		data, _ := ResizeImageIfNeeded(d, filename)
		fileData = data
	} else {
		var upload models.Upload
		if err := s.DB.Uploads().FindOne(ctx, bson.M{"filename": filename}).Decode(&upload); err != nil {
			return "", fmt.Errorf("upload not found: %s", filename)
		}
		data, _ := ResizeImageIfNeeded(upload.Data, filename)
		fileData = data
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("media", filename)
	if err != nil {
		return "", err
	}
	part.Write(fileData)
	writer.Close()

	endpoint := twitterUploadBase + "/media/upload.json"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", s.buildOAuthHeader("POST", endpoint, nil, account))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitter media upload failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		MediaIDString string `json:"media_id_string"`
	}
	json.Unmarshal(body, &result)
	if result.MediaIDString == "" {
		return "", fmt.Errorf("twitter media upload returned empty media_id")
	}
	return result.MediaIDString, nil
}

func (s *TwitterService) getAccount(ctx context.Context, accountID string) (*models.SocialAccount, error) {
	filter := bson.M{"platform": models.PlatformTwitter, "isActive": true}
	if accountID != "" {
		id, err := primitive.ObjectIDFromHex(accountID)
		if err != nil {
			return nil, fmt.Errorf("invalid twitter account ID: %s", accountID)
		}
		filter["_id"] = id
	}

	var account models.SocialAccount
	err := s.DB.SocialAccounts().FindOne(ctx, filter).Decode(&account)
	return &account, err
}

// buildOAuthHeader constructs the OAuth 1.0a Authorization header.
// queryParams should contain URL query parameters (included in the signature base string).
// JSON request bodies are NOT included in the signature per OAuth 1.0a spec.
func (s *TwitterService) buildOAuthHeader(method, rawURL string, queryParams url.Values, account *models.SocialAccount) string {
	nonce := generateOAuthNonce()
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	oauthParams := map[string]string{
		"oauth_consumer_key":     account.ConsumerKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        timestamp,
		"oauth_token":            account.AccessToken,
		"oauth_version":          "1.0",
	}

	// Collect all params for signature base string
	allParams := url.Values{}
	for k, v := range oauthParams {
		allParams.Set(k, v)
	}
	for k, vs := range queryParams {
		for _, v := range vs {
			allParams.Add(k, v)
		}
	}

	var paramParts []string
	for k, vs := range allParams {
		for _, v := range vs {
			paramParts = append(paramParts, percentEncode(k)+"="+percentEncode(v))
		}
	}
	sort.Strings(paramParts)
	paramStr := strings.Join(paramParts, "&")

	baseStr := strings.ToUpper(method) + "&" + percentEncode(rawURL) + "&" + percentEncode(paramStr)
	signingKey := percentEncode(account.ConsumerSecret) + "&" + percentEncode(account.AccessTokenSecret)

	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(baseStr))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	oauthParams["oauth_signature"] = sig

	var headerParts []string
	for k, v := range oauthParams {
		headerParts = append(headerParts, fmt.Sprintf(`%s="%s"`, percentEncode(k), percentEncode(v)))
	}
	sort.Strings(headerParts)

	return "OAuth " + strings.Join(headerParts, ", ")
}

func generateOAuthNonce() string {
	b := make([]byte, 32)
	rand.Read(b)
	// Use only alphanumeric chars per OAuth spec
	return base64.RawURLEncoding.EncodeToString(b)
}

// percentEncode encodes a string per RFC 3986.
func percentEncode(s string) string {
	encoded := url.QueryEscape(s)
	// QueryEscape uses + for spaces; OAuth needs %20
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	return encoded
}
