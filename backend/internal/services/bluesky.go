package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

const bskyChatAPI = "https://api.bsky.chat"

// chatRequest makes a request to the Bluesky chat API directly.
func (s *BlueskyService) chatRequest(ctx context.Context, session *bskySession, method, endpoint string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, bskyChatAPI+"/xrpc/"+endpoint, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+session.AccessJwt)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("chat API %s returned %d: %s", endpoint, resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// BSkyConvo represents a Bluesky chat conversation.
type BSkyConvo struct {
	ID          string           `json:"id"`
	Members     []BSkyDMProfile  `json:"members"`
	LastMessage json.RawMessage  `json:"lastMessage"`
	UnreadCount int              `json:"unreadCount"`
}

// ParseLastMessage extracts the last message from the raw JSON union type.
func (c *BSkyConvo) ParseLastMessage() *BSkyDMMessage {
	if len(c.LastMessage) == 0 {
		return nil
	}
	var msg BSkyDMMessage
	if err := json.Unmarshal(c.LastMessage, &msg); err != nil {
		return nil
	}
	return &msg
}

// BSkyDMProfile represents a user profile in a DM conversation.
type BSkyDMProfile struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
}

// BSkyDMMessage represents a single chat message.
type BSkyDMMessage struct {
	Type   string       `json:"$type"`
	ID     string       `json:"id"`
	Text   string       `json:"text"`
	Sender BSkyDMSender `json:"sender"`
	SentAt string       `json:"sentAt"`
}

// BSkyDMSender identifies who sent a chat message.
type BSkyDMSender struct {
	DID string `json:"did"`
}

// ListConvos returns recent Bluesky DM conversations.
func (s *BlueskyService) ListConvos(ctx context.Context, accountID string) ([]BSkyConvo, *models.SocialAccount, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}

	session, err := s.createSession(account)
	if err != nil {
		return nil, nil, err
	}

	// Ensure the account has the DID stored
	if account.DID == "" && session.DID != "" {
		account.DID = session.DID
		s.DB.SocialAccounts().UpdateOne(ctx,
			bson.M{"_id": account.ID},
			bson.M{"$set": bson.M{"did": session.DID}},
		)
	}

	data, err := s.chatRequest(ctx, session, "GET", "chat.bsky.convo.listConvos?limit=50", nil)
	if err != nil {
		return nil, nil, err
	}

	log.Printf("ListConvos: raw response length=%d account=%s did=%s", len(data), account.AccountName, account.DID)

	var result struct {
		Convos []BSkyConvo `json:"convos"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("ListConvos: unmarshal error: %v body=%s", err, string(data))
		return nil, nil, err
	}

	log.Printf("ListConvos: found %d conversations", len(result.Convos))
	return result.Convos, account, nil
}

// GetConvoMessages returns messages from a specific conversation.
func (s *BlueskyService) GetConvoMessages(ctx context.Context, convoID, accountID string) ([]BSkyDMMessage, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	session, err := s.createSession(account)
	if err != nil {
		return nil, err
	}

	data, err := s.chatRequest(ctx, session, "GET",
		fmt.Sprintf("chat.bsky.convo.getMessages?convoId=%s&limit=50", convoID), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Messages []BSkyDMMessage `json:"messages"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result.Messages, nil
}

// SendChatMessage sends a DM reply in an existing Bluesky conversation.
func (s *BlueskyService) SendChatMessage(ctx context.Context, convoID, text, accountID string) error {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return err
	}

	session, err := s.createSession(account)
	if err != nil {
		return err
	}

	body := map[string]interface{}{
		"convoId": convoID,
		"message": map[string]string{
			"text": text,
		},
	}

	_, err = s.chatRequest(ctx, session, "POST", "chat.bsky.convo.sendMessage", body)
	return err
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

// BSkyNotification represents a Bluesky notification.
type BSkyNotification struct {
	URI       string `json:"uri"`
	CID       string `json:"cid"`
	Reason    string `json:"reason"`
	IsRead    bool   `json:"isRead"`
	IndexedAt string `json:"indexedAt"`
	Author    struct {
		DID         string `json:"did"`
		Handle      string `json:"handle"`
		DisplayName string `json:"displayName"`
		Avatar      string `json:"avatar"`
	} `json:"author"`
	Record struct {
		Type      string `json:"$type"`
		Text      string `json:"text"`
		CreatedAt string `json:"createdAt"`
		Reply     *struct {
			Parent struct {
				URI string `json:"uri"`
				CID string `json:"cid"`
			} `json:"parent"`
			Root struct {
				URI string `json:"uri"`
			} `json:"root"`
		} `json:"reply"`
	} `json:"record"`
}

// FetchNotifications retrieves recent Bluesky notifications (replies/mentions).
func (s *BlueskyService) FetchNotifications(ctx context.Context, accountID string) ([]BSkyNotification, *models.SocialAccount, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}

	session, err := s.createSession(account)
	if err != nil {
		return nil, nil, err
	}

	if account.DID == "" && session.DID != "" {
		account.DID = session.DID
		s.DB.SocialAccounts().UpdateOne(ctx,
			bson.M{"_id": account.ID},
			bson.M{"$set": bson.M{"did": session.DID}},
		)
	}

	req, _ := http.NewRequestWithContext(ctx, "GET",
		account.PDSHost+"/xrpc/app.bsky.notification.listNotifications?limit=50", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessJwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Notifications []BSkyNotification `json:"notifications"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, err
	}

	return result.Notifications, account, nil
}

// ResolveURI fetches a Bluesky post record to get its CID.
func (s *BlueskyService) ResolveURI(ctx context.Context, uri, accountID string) (cid string, err error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", err
	}

	session, err := s.createSession(account)
	if err != nil {
		return "", err
	}

	// Parse AT URI: at://did/collection/rkey
	parts := strings.SplitN(strings.TrimPrefix(uri, "at://"), "/", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid AT URI: %s", uri)
	}

	endpoint := fmt.Sprintf("%s/xrpc/com.atproto.repo.getRecord?repo=%s&collection=%s&rkey=%s",
		account.PDSHost, parts[0], parts[1], parts[2])
	req, _ := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessJwt)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		CID string `json:"cid"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.CID, nil
}

// LikePost creates an app.bsky.feed.like record for the given post URI/CID.
func (s *BlueskyService) LikePost(ctx context.Context, uri, cid, accountID string) error {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("no active Bluesky account: %w", err)
	}

	session, err := s.createSession(account)
	if err != nil {
		return fmt.Errorf("auth failed: %w", err)
	}

	body := map[string]interface{}{
		"repo":       session.DID,
		"collection": "app.bsky.feed.like",
		"record": map[string]interface{}{
			"$type":     "app.bsky.feed.like",
			"subject":   map[string]string{"uri": uri, "cid": cid},
			"createdAt": time.Now().UTC().Format(time.RFC3339),
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		account.PDSHost+"/xrpc/com.atproto.repo.createRecord",
		bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer "+session.AccessJwt)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("like failed (%d): %s", resp.StatusCode, string(respBody))
	}
	return nil
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
