package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type BlueskyService struct {
	DB        *database.MongoDB
	UploadDir string
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

func (s *BlueskyService) Post(ctx context.Context, content string, imageURLs []string, accountID string) (uri, cid string, err error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", "", fmt.Errorf("no active Bluesky account: %w", err)
	}

	session, err := s.createSession(account)
	if err != nil {
		return "", "", fmt.Errorf("auth failed: %w", err)
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
		images, uploadErr := s.uploadImages(session, account.PDSHost, imageURLs)
		if uploadErr == nil && len(images) > 0 {
			record["embed"] = map[string]interface{}{
				"$type":  "app.bsky.embed.images",
				"images": images,
			}
		}
	}

	uri, cid, err = s.createRecord(ctx, session, account.PDSHost, record)
	return uri, cid, err
}

// PostReply posts a reply to an existing Bluesky post.
func (s *BlueskyService) PostReply(ctx context.Context, content, parentURI, parentCID, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active Bluesky account: %w", err)
	}

	session, err := s.createSession(account)
	if err != nil {
		return "", fmt.Errorf("auth failed: %w", err)
	}

	ref := map[string]string{"uri": parentURI, "cid": parentCID}
	record := map[string]interface{}{
		"$type":     "app.bsky.feed.post",
		"text":      content,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"reply": map[string]interface{}{
			"root":   ref,
			"parent": ref,
		},
	}

	facets := s.parseFacets(content)
	if len(facets) > 0 {
		record["facets"] = facets
	}

	uri, _, err := s.createRecord(ctx, session, account.PDSHost, record)
	return uri, err
}

func (s *BlueskyService) createRecord(ctx context.Context, session *bskySession, pdsHost string, record map[string]interface{}) (uri, cid string, err error) {
	body := map[string]interface{}{
		"repo":       session.DID,
		"collection": "app.bsky.feed.post",
		"record":     record,
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		pdsHost+"/xrpc/com.atproto.repo.createRecord",
		bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer "+session.AccessJwt)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("post failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.URI, result.CID, nil
}

func (s *BlueskyService) getAccount(ctx context.Context, accountID string) (*models.SocialAccount, error) {
	if accountID == "" {
		return nil, fmt.Errorf("bluesky account ID is required")
	}
	id, err := primitive.ObjectIDFromHex(accountID)
	if err != nil {
		return nil, fmt.Errorf("invalid bluesky account ID: %s", accountID)
	}

	var account models.SocialAccount
	err = s.DB.SocialAccounts().FindOne(ctx, bson.M{
		"platform": models.PlatformBluesky,
		"isActive": true,
		"_id":      id,
	}).Decode(&account)
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

// FetchProfile authenticates and returns display name + avatar URL for a Bluesky account.
// It also sets the account's DID as a side effect.
func (s *BlueskyService) FetchProfile(account *models.SocialAccount) (displayName, avatarURL string, err error) {
	session, err := s.createSession(account)
	if err != nil {
		return "", "", err
	}

	// Store the DID on the account so it can be persisted
	if session.DID != "" {
		account.DID = session.DID
	}

	req, _ := http.NewRequest("GET",
		account.PDSHost+"/xrpc/app.bsky.actor.getProfile?actor="+session.DID, nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessJwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var profile struct {
		DisplayName string `json:"displayName"`
		Avatar      string `json:"avatar"`
	}
	json.NewDecoder(resp.Body).Decode(&profile)
	return profile.DisplayName, profile.Avatar, nil
}

func (s *BlueskyService) uploadImages(session *bskySession, pdsHost string, imageURLs []string) ([]map[string]interface{}, error) {
	var images []map[string]interface{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, url := range imageURLs {
		if len(images) >= 4 { // Bluesky max 4 images
			break
		}

		// Read upload data from disk first, then fall back to MongoDB.
		filename := strings.TrimPrefix(url, "/api/uploads/")
		var fileData []byte
		diskPath := filepath.Join(s.UploadDir, filename)
		if d, readErr := os.ReadFile(diskPath); readErr == nil {
			fileData = d
		} else {
			var upload models.Upload
			err := s.DB.Uploads().FindOne(ctx, bson.M{"filename": filename}).Decode(&upload)
			if err != nil {
				continue
			}
			fileData = upload.Data
		}

		// Resize if over Bluesky's 1MB blob limit
		data, contentType := ResizeImageIfNeeded(fileData, filename)

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

// BSkyFeedItem represents a single post from the user's Bluesky feed.
type BSkyFeedItem struct {
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

// FetchFeed retrieves the authenticated user's own Bluesky post feed.
func (s *BlueskyService) FetchFeed(ctx context.Context, accountID string) ([]BSkyFeedItem, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("no active Bluesky account: %w", err)
	}

	session, err := s.createSession(account)
	if err != nil {
		return nil, fmt.Errorf("auth failed: %w", err)
	}

	actor := session.DID
	if actor == "" {
		actor = account.AccountName
	}

	req, _ := http.NewRequestWithContext(ctx, "GET",
		account.PDSHost+"/xrpc/app.bsky.feed.getAuthorFeed?actor="+actor+"&limit=50&filter=posts_no_replies", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessJwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Feed []struct {
			Post struct {
				URI    string `json:"uri"`
				CID    string `json:"cid"`
				Author struct {
					Handle string `json:"handle"`
				} `json:"author"`
				Record struct {
					Text      string `json:"text"`
					CreatedAt string `json:"createdAt"`
				} `json:"record"`
				Embed *struct {
					Type   string `json:"$type"`
					Images []struct {
						Thumb    string `json:"thumb"`
						Fullsize string `json:"fullsize"`
					} `json:"images"`
				} `json:"embed"`
				LikeCount  int `json:"likeCount"`
				ReplyCount int `json:"replyCount"`
			} `json:"post"`
		} `json:"feed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	items := make([]BSkyFeedItem, 0, len(result.Feed))
	for _, entry := range result.Feed {
		p := entry.Post
		// Build permalink from URI: at://did/app.bsky.feed.post/rkey -> https://bsky.app/profile/handle/post/rkey
		permalink := ""
		if parts := strings.SplitN(p.URI, "/", 5); len(parts) == 5 {
			rkey := parts[4]
			permalink = fmt.Sprintf("https://bsky.app/profile/%s/post/%s", p.Author.Handle, rkey)
		}

		mediaType := "TEXT"
		mediaURL := ""
		thumbnailURL := ""
		if p.Embed != nil && len(p.Embed.Images) > 0 {
			mediaType = "IMAGE"
			mediaURL = p.Embed.Images[0].Fullsize
			thumbnailURL = p.Embed.Images[0].Thumb
		}

		items = append(items, BSkyFeedItem{
			ID:            p.URI,
			Caption:       p.Record.Text,
			MediaType:     mediaType,
			MediaURL:      mediaURL,
			ThumbnailURL:  thumbnailURL,
			Permalink:     permalink,
			Timestamp:     p.Record.CreatedAt,
			LikeCount:     p.LikeCount,
			CommentsCount: p.ReplyCount,
		})
	}
	return items, nil
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
