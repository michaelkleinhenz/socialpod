package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BlueskyService struct {
	DB *database.MongoDB
}

type bskySession struct {
	DID        string `json:"did"`
	AccessJwt  string `json:"accessJwt"`
	RefreshJwt string `json:"refreshJwt"`
}

type bskyEmbed struct {
	Type   string      `json:"$type"`
	Images []bskyImage `json:"images,omitempty"`
}

type bskyImage struct {
	Alt   string      `json:"alt"`
	Image interface{} `json:"image"`
}

func (s *BlueskyService) Post(ctx context.Context, content string, imageURLs []string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active Bluesky account: %w", err)
	}

	session, err := s.createSession(account)
	if err != nil {
		return "", fmt.Errorf("auth failed: %w", err)
	}

	// Build post record
	record := map[string]interface{}{
		"$type":     "app.bsky.feed.post",
		"text":      content,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	// Parse facets (mentions, links, hashtags)
	facets := s.parseFacets(content)
	if len(facets) > 0 {
		record["facets"] = facets
	}

	// Upload images if present
	if len(imageURLs) > 0 {
		images, err := s.uploadImages(session, account.PDSHost, imageURLs)
		if err == nil && len(images) > 0 {
			record["embed"] = map[string]interface{}{
				"$type":  "app.bsky.embed.images",
				"images": images,
			}
		}
	}

	body := map[string]interface{}{
		"repo":       session.DID,
		"collection": "app.bsky.feed.post",
		"record":     record,
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		account.PDSHost+"/xrpc/com.atproto.repo.createRecord",
		bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer "+session.AccessJwt)
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
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.URI, nil
}

func (s *BlueskyService) getAccount(ctx context.Context, accountID string) (*models.SocialAccount, error) {
	filter := bson.M{"platform": models.PlatformBluesky, "isActive": true}
	if accountID != "" {
		if id, err := primitive.ObjectIDFromHex(accountID); err == nil {
			filter["_id"] = id
		}
	}

	var account models.SocialAccount
	err := s.DB.SocialAccounts().FindOne(ctx, filter).Decode(&account)
	return &account, err
}

func (s *BlueskyService) createSession(account *models.SocialAccount) (*bskySession, error) {
	body := map[string]string{
		"identifier": account.AccountName,
		"password":   account.AppPassword,
	}
	jsonBody, _ := json.Marshal(body)

	resp, err := http.Post(account.PDSHost+"/xrpc/com.atproto.server.createSession",
		"application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("auth failed with status %d", resp.StatusCode)
	}

	var session bskySession
	json.NewDecoder(resp.Body).Decode(&session)
	return &session, nil
}

func (s *BlueskyService) uploadImages(session *bskySession, pdsHost string, imageURLs []string) ([]map[string]interface{}, error) {
	var images []map[string]interface{}
	for _, url := range imageURLs {
		if len(images) >= 4 { // Bluesky max 4 images
			break
		}

		// Read local file
		localPath := strings.TrimPrefix(url, "/api/uploads/")
		uploadDir := os.Getenv("UPLOAD_DIR")
		if uploadDir == "" {
			uploadDir = "./uploads"
		}
		filePath := uploadDir + "/" + localPath

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		// Resize if over Bluesky's 1MB blob limit
		data, contentType := ResizeImageIfNeeded(data, localPath)

		req, _ := http.NewRequest("POST", pdsHost+"/xrpc/com.atproto.repo.uploadBlob",
			bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+session.AccessJwt)
		req.Header.Set("Content-Type", contentType)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}

		var result struct {
			Blob interface{} `json:"blob"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if result.Blob != nil {
			images = append(images, map[string]interface{}{
				"alt":   "",
				"image": result.Blob,
			})
		}
	}

	return images, nil
}

func (s *BlueskyService) parseFacets(text string) []map[string]interface{} {
	var facets []map[string]interface{}

	// Parse hashtags
	words := strings.Fields(text)
	pos := 0
	for _, word := range words {
		idx := strings.Index(text[pos:], word)
		if idx < 0 {
			continue
		}
		start := pos + idx
		end := start + len(word)
		pos = end

		if strings.HasPrefix(word, "#") && len(word) > 1 {
			tag := strings.TrimPrefix(word, "#")
			// Remove trailing punctuation
			tag = strings.TrimRight(tag, ".,!?;:")
			facets = append(facets, map[string]interface{}{
				"index": map[string]int{
					"byteStart": start,
					"byteEnd":   start + 1 + len(tag),
				},
				"features": []map[string]interface{}{
					{
						"$type": "app.bsky.richtext.facet#tag",
						"tag":   tag,
					},
				},
			})
		}
	}

	return facets
}
