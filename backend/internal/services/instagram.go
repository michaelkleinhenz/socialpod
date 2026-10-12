package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
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

type igError struct {
	Message      string `json:"message"`
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	FBTraceID    string `json:"fbtrace_id"`
}

func (e *igError) Error() string {
	return fmt.Sprintf("IG error (code=%d, subcode=%d, type=%s, trace=%s): %s",
		e.Code, e.ErrorSubcode, e.Type, e.FBTraceID, e.Message)
}

const igGraphAPI = "https://graph.instagram.com/v25.0"
const igGraphAPIUnversioned = "https://graph.instagram.com"

// MediaExtras are the optional Instagram-only additions to a feed post or
// reel: collaborators invited to co-author it and accounts tagged on it.
type MediaExtras struct {
	Collaborators []string
	UserTags      []string
}

func (e MediaExtras) empty() bool { return len(e.Collaborators) == 0 && len(e.UserTags) == 0 }

const (
	igMaxCollaborators = 3
	igMaxUserTags      = 20
)

var igUsernameRe = regexp.MustCompile(`^[A-Za-z0-9._]{1,30}$`)

// NormalizeIGUsernames strips "@", drops anything that is not a valid
// Instagram username, removes duplicates (case-insensitively) and the account
// itself, and caps the list at max entries.
func NormalizeIGUsernames(names []string, self string, max int) []string {
	seen := map[string]bool{strings.ToLower(strings.TrimPrefix(self, "@")): true}
	var out []string
	for _, n := range names {
		n = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(n), "@"))
		key := strings.ToLower(n)
		if n == "" || seen[key] || !igUsernameRe.MatchString(n) {
			continue
		}
		seen[key] = true
		out = append(out, n)
		if len(out) == max {
			break
		}
	}
	return out
}

// collaboratorsParam encodes usernames the way the media endpoint expects.
func collaboratorsParam(names []string) string {
	b, _ := json.Marshal(names)
	return string(b)
}

// userTagsParam encodes image tags. Instagram wants a position for each tag on
// an image, so the tags are spread along the bottom edge where they cover the
// least; a reel takes usernames only.
func userTagsParam(names []string, withPosition bool) string {
	tags := make([]map[string]any, 0, len(names))
	for i, n := range names {
		tag := map[string]any{"username": n}
		if withPosition {
			tag["x"] = math.Round(float64(i+1)/float64(len(names)+1)*100) / 100
			if i%2 == 0 {
				tag["y"] = 0.9
			} else {
				tag["y"] = 0.95
			}
		}
		tags = append(tags, tag)
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

// createContainer creates a media container and returns its ID.
func (s *InstagramService) createContainer(account *models.SocialAccount, params url.Values) (string, error) {
	params.Set("access_token", account.AccessToken)
	resp, err := http.PostForm(fmt.Sprintf("%s/%s/media", igGraphAPI, account.IGUserID), params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("IG container response: status=%d body=%s", resp.StatusCode, string(body))

	var container struct {
		ID    string   `json:"id"`
		Error *igError `json:"error"`
	}
	json.Unmarshal(body, &container)
	if container.Error != nil {
		return "", container.Error
	}
	if container.ID == "" {
		return "", fmt.Errorf("IG returned no container ID (HTTP %d)", resp.StatusCode)
	}
	return container.ID, nil
}

// createContainerWithExtras creates a container with the given collaborator
// and tag parameters. A username Instagram does not know makes it refuse the
// whole container, so when that happens the container is created again
// without them: the post still goes out, and the returned warning says what
// was dropped.
func (s *InstagramService) createContainerWithExtras(account *models.SocialAccount, params url.Values, extras url.Values) (string, string, error) {
	if len(extras) == 0 {
		id, err := s.createContainer(account, params)
		return id, "", err
	}
	withExtras := url.Values{}
	for k, v := range params {
		withExtras[k] = v
	}
	for k, v := range extras {
		withExtras[k] = v
	}
	id, err := s.createContainer(account, withExtras)
	if err == nil {
		return id, "", nil
	}
	log.Printf("IG container with collaborators/tags failed, retrying without them: %v", err)
	id, retryErr := s.createContainer(account, params)
	if retryErr != nil {
		return "", "", retryErr
	}
	var dropped []string
	if extras.Has("collaborators") {
		dropped = append(dropped, "collaborators")
	}
	if extras.Has("user_tags") {
		dropped = append(dropped, "tags")
	}
	return id, fmt.Sprintf("Posted without %s: %v", strings.Join(dropped, " and "), err), nil
}

// Post publishes a feed post (single image or carousel) and returns the media
// ID plus a warning when collaborators or tags had to be dropped.
func (s *InstagramService) Post(ctx context.Context, content string, imageURLs []string, accountID string, extras MediaExtras) (string, string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", "", fmt.Errorf("no active Instagram account: %w", err)
	}

	if len(imageURLs) == 0 {
		return "", "", fmt.Errorf("Instagram requires at least one image")
	}

	// Get app settings for the public URL
	var settings models.AppSettings
	s.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)
	baseURL := settings.AppURL

	extras.Collaborators = NormalizeIGUsernames(extras.Collaborators, account.AccountName, igMaxCollaborators)
	extras.UserTags = NormalizeIGUsernames(extras.UserTags, account.AccountName, igMaxUserTags)

	if len(imageURLs) == 1 {
		fullURL := imageURLs[0]
		if !strings.HasPrefix(fullURL, "http") {
			fullURL = baseURL + fullURL
		}
		return s.postSingleImage(ctx, account, content, fullURL, extras)
	}
	return s.postCarousel(ctx, account, content, imageURLs, baseURL, extras)
}

func (s *InstagramService) postSingleImage(ctx context.Context, account *models.SocialAccount, caption, mediaURL string, extras MediaExtras) (string, string, error) {
	// Step 1: Create media container
	params := url.Values{
		"caption": {caption},
	}

	lowerURL := strings.ToLower(mediaURL)
	isVideo := strings.HasSuffix(lowerURL, ".mp4") || strings.HasSuffix(lowerURL, ".mov")
	if isVideo {
		params.Set("media_type", "REELS")
		params.Set("video_url", mediaURL)
	} else {
		params.Set("image_url", mediaURL)
	}

	extraParams := url.Values{}
	if len(extras.Collaborators) > 0 {
		extraParams.Set("collaborators", collaboratorsParam(extras.Collaborators))
	}
	if len(extras.UserTags) > 0 {
		extraParams.Set("user_tags", userTagsParam(extras.UserTags, !isVideo))
	}

	log.Printf("IG post create: image_url=%s igUser=%s", mediaURL, account.IGUserID)

	containerID, warning, err := s.createContainerWithExtras(account, params, extraParams)
	if err != nil {
		return "", "", err
	}

	// Step 2: Publish
	id, err := s.publishContainer(account, containerID)
	return id, warning, err
}

func (s *InstagramService) postCarousel(ctx context.Context, account *models.SocialAccount, caption string, imageURLs []string, baseURL string, extras MediaExtras) (string, string, error) {
	var childIDs []string
	var warnings []string

	// Create child containers. Tags go on the first image: Instagram takes
	// them per carousel item, not on the carousel itself.
	tagsPlaced := false
	for _, imgURL := range imageURLs {
		if len(childIDs) >= 10 { // IG carousel max
			break
		}
		fullURL := imgURL
		if !strings.HasPrefix(imgURL, "http") {
			fullURL = baseURL + imgURL
		}

		params := url.Values{
			"is_carousel_item": {"true"},
		}

		lowerURL := strings.ToLower(fullURL)
		isVideo := strings.HasSuffix(lowerURL, ".mp4") || strings.HasSuffix(lowerURL, ".mov")
		if isVideo {
			params.Set("video_url", fullURL)
		} else {
			params.Set("image_url", fullURL)
		}

		extraParams := url.Values{}
		if !tagsPlaced && !isVideo && len(extras.UserTags) > 0 {
			extraParams.Set("user_tags", userTagsParam(extras.UserTags, true))
			tagsPlaced = true
		}

		log.Printf("IG carousel child create: image_url=%s", fullURL)

		id, warning, err := s.createContainerWithExtras(account, params, extraParams)
		if err != nil {
			log.Printf("IG carousel child error: %v", err)
			continue
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		childIDs = append(childIDs, id)
	}

	if len(childIDs) == 0 {
		return "", "", fmt.Errorf("no carousel items created")
	}

	// Create carousel container
	params := url.Values{
		"media_type": {"CAROUSEL"},
		"caption":    {caption},
		"children":   {strings.Join(childIDs, ",")},
	}
	extraParams := url.Values{}
	if len(extras.Collaborators) > 0 {
		extraParams.Set("collaborators", collaboratorsParam(extras.Collaborators))
	}

	containerID, warning, err := s.createContainerWithExtras(account, params, extraParams)
	if err != nil {
		return "", "", err
	}
	if warning != "" {
		warnings = append(warnings, warning)
	}

	id, err := s.publishContainer(account, containerID)
	return id, strings.Join(warnings, "; "), err
}

func (s *InstagramService) waitForContainer(account *models.SocialAccount, containerID string) error {
	// Video containers can take several minutes to process on Instagram's side.
	// Poll for up to 5 minutes with increasing intervals.
	maxAttempts := 60
	for i := 0; i < maxAttempts; i++ {
		resp, err := http.Get(fmt.Sprintf("%s/%s?%s", igGraphAPIUnversioned, containerID, url.Values{
			"fields":       {"status_code,status"},
			"access_token": {account.AccessToken},
		}.Encode()))
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

	body, _ := io.ReadAll(resp.Body)
	log.Printf("IG publish response: status=%d body=%s", resp.StatusCode, string(body))

	var result struct {
		ID    string   `json:"id"`
		Error *igError `json:"error"`
	}
	json.Unmarshal(body, &result)
	if result.Error != nil {
		return "", fmt.Errorf("publish error: %w", result.Error)
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

	log.Printf("IG story create: url=%s igUser=%s", fullURL, account.IGUserID)
	resp, err := http.PostForm(fmt.Sprintf("%s/%s/media", igGraphAPI, account.IGUserID), params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("IG story container response: %s", string(body))

	var container struct {
		ID    string   `json:"id"`
		Error *igError `json:"error"`
	}
	json.Unmarshal(body, &container)
	if container.Error != nil {
		return "", fmt.Errorf("IG story error: %w", container.Error)
	}

	return s.publishContainer(account, container.ID)
}

// PostReel publishes an Instagram Reel. mediaURL must be a publicly reachable
// URL pointing to an MP4 video file.
func (s *InstagramService) PostReel(ctx context.Context, mediaURL string, caption string, accountID string, extras MediaExtras) (string, string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", "", fmt.Errorf("no active Instagram account: %w", err)
	}

	var settings models.AppSettings
	s.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)

	fullURL := mediaURL
	if !strings.HasPrefix(mediaURL, "http") {
		fullURL = settings.AppURL + mediaURL
	}

	params := url.Values{
		"media_type": {"REELS"},
		"video_url":  {fullURL},
		"caption":    {caption},
	}

	extraParams := url.Values{}
	if c := NormalizeIGUsernames(extras.Collaborators, account.AccountName, igMaxCollaborators); len(c) > 0 {
		extraParams.Set("collaborators", collaboratorsParam(c))
	}
	if t := NormalizeIGUsernames(extras.UserTags, account.AccountName, igMaxUserTags); len(t) > 0 {
		extraParams.Set("user_tags", userTagsParam(t, false))
	}

	log.Printf("IG reel create: url=%s", fullURL)
	containerID, warning, err := s.createContainerWithExtras(account, params, extraParams)
	if err != nil {
		return "", "", fmt.Errorf("IG reel error: %w", err)
	}

	id, err := s.publishContainer(account, containerID)
	return id, warning, err
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
		ID    string   `json:"id"`
		Error *igError `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error != nil {
		return fmt.Errorf("IG comment error: %w", result.Error)
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

	resp, err := http.Get(fmt.Sprintf("%s/%s/media?%s", igGraphAPIUnversioned, account.IGUserID, url.Values{
		"fields":       {"id,caption,media_type,media_url,thumbnail_url,permalink,timestamp,like_count,comments_count"},
		"access_token": {account.AccessToken},
	}.Encode()))
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
	if accountID == "" {
		return nil, fmt.Errorf("instagram account ID is required")
	}
	id, err := primitive.ObjectIDFromHex(accountID)
	if err != nil || id == primitive.NilObjectID {
		return nil, fmt.Errorf("invalid instagram account ID: %s", accountID)
	}
	var account models.SocialAccount
	if err := s.DB.SocialAccounts().FindOne(ctx, bson.M{
		"platform": models.PlatformInstagram,
		"isActive": true,
		"_id":      id,
	}).Decode(&account); err != nil {
		return nil, fmt.Errorf("instagram account not found: %w", err)
	}
	return &account, nil
}

// FetchProfile retrieves the current profile picture URL for an existing account.
func (s *InstagramService) FetchProfile(account *models.SocialAccount) (displayName, avatarURL string, err error) {
	if account.AccessToken == "" || account.IGUserID == "" {
		return "", "", fmt.Errorf("account missing access token or user ID")
	}

	resp, err := http.Get(fmt.Sprintf("%s/%s?%s", igGraphAPIUnversioned, account.IGUserID, url.Values{
		"fields":       {"username,profile_picture_url"},
		"access_token": {account.AccessToken},
	}.Encode()))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var profile struct {
		Username          string `json:"username"`
		ProfilePictureURL string `json:"profile_picture_url"`
	}
	json.NewDecoder(resp.Body).Decode(&profile)

	dn := profile.Username
	if dn == "" {
		dn = account.AccountName
	}
	return dn, profile.ProfilePictureURL, nil
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

	shortBody, _ := io.ReadAll(resp.Body)
	log.Printf("IG short-lived token response: status=%d body=%s", resp.StatusCode, string(shortBody))

	var shortToken struct {
		AccessToken string `json:"access_token"`
		UserID      int64  `json:"user_id"`
	}
	json.Unmarshal(shortBody, &shortToken)

	if shortToken.AccessToken == "" {
		return nil, fmt.Errorf("failed to get access token")
	}

	// Exchange for long-lived token.
	// Try graph.instagram.com first (documented host), then graph.facebook.com
	// as fallback (Meta docs state both hosts serve all endpoints).
	token := shortToken.AccessToken
	var tokenExpiresIn int64
	llParams := url.Values{
		"grant_type":    {"ig_exchange_token"},
		"client_secret": {clientSecret},
		"access_token":  {shortToken.AccessToken},
	}
	for _, host := range []string{igGraphAPIUnversioned, "https://graph.facebook.com"} {
		llURL := fmt.Sprintf("%s/access_token?%s", host, llParams.Encode())
		log.Printf("IG long-lived token trying: %s", strings.Replace(llURL, clientSecret, "***", 1))
		llResp, llErr := http.Get(llURL)
		if llErr != nil {
			log.Printf("IG long-lived token http error (%s): %v", host, llErr)
			continue
		}
		llBody, _ := io.ReadAll(llResp.Body)
		llResp.Body.Close()
		log.Printf("IG long-lived token response (%s): status=%d body=%s", host, llResp.StatusCode, string(llBody))
		var longToken struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		}
		json.Unmarshal(llBody, &longToken)
		if longToken.AccessToken != "" {
			token = longToken.AccessToken
			tokenExpiresIn = longToken.ExpiresIn
			break
		}
	}

	igUserID := fmt.Sprintf("%d", shortToken.UserID)

	// Fetch profile: try both hosts and both /me and /{user_id} endpoints.
	var profile struct {
		ID                string   `json:"id"`
		Username          string   `json:"username"`
		ProfilePictureURL string   `json:"profile_picture_url"`
		Error             *igError `json:"error"`
	}
	profileEndpoints := []string{
		fmt.Sprintf("%s/me", igGraphAPIUnversioned),
		fmt.Sprintf("%s/%s", igGraphAPIUnversioned, igUserID),
		fmt.Sprintf("https://graph.facebook.com/me"),
		fmt.Sprintf("https://graph.facebook.com/%s", igUserID),
	}
	profileParams := url.Values{
		"fields":       {"id,username,profile_picture_url"},
		"access_token": {token},
	}
	for _, ep := range profileEndpoints {
		reqURL := fmt.Sprintf("%s?%s", ep, profileParams.Encode())
		log.Printf("IG profile trying: %s", ep)
		pResp, pErr := http.Get(reqURL)
		if pErr != nil {
			log.Printf("IG profile http error (%s): %v", ep, pErr)
			continue
		}
		pBody, _ := io.ReadAll(pResp.Body)
		pResp.Body.Close()
		log.Printf("IG profile response (%s): status=%d body=%s", ep, pResp.StatusCode, string(pBody))
		var p struct {
			ID                string   `json:"id"`
			Username          string   `json:"username"`
			ProfilePictureURL string   `json:"profile_picture_url"`
			Error             *igError `json:"error"`
		}
		json.Unmarshal(pBody, &p)
		if p.Username != "" || p.ID != "" {
			profile = p
			break
		}
	}

	if profile.ID != "" {
		igUserID = profile.ID
	}

	username := profile.Username

	account := &models.SocialAccount{
		Platform:    models.PlatformInstagram,
		AccountName: username,
		DisplayName: username,
		AccessToken: token,
		IGUserID:    igUserID,
		AvatarURL:   profile.ProfilePictureURL,
		IsActive:    true,
	}
	if tokenExpiresIn > 0 {
		expiry := time.Now().Add(time.Duration(tokenExpiresIn) * time.Second)
		account.TokenExpiry = &expiry
	}

	return account, nil
}

// RefreshLongLivedToken exchanges the current long-lived token for a new one.
// Instagram long-lived tokens can be refreshed when they are at least 24 hours
// old and not yet expired.
func (s *InstagramService) RefreshLongLivedToken(ctx context.Context, account *models.SocialAccount) error {
	resp, err := http.Get(fmt.Sprintf("%s/refresh_access_token?%s", igGraphAPIUnversioned, url.Values{
		"grant_type":   {"ig_refresh_token"},
		"access_token": {account.AccessToken},
	}.Encode()))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("IG token refresh response: status=%d body=%s", resp.StatusCode, string(body))

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	json.Unmarshal(body, &tokenResp)

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("IG token refresh failed: empty access_token")
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	_, err = s.DB.SocialAccounts().UpdateOne(ctx, bson.M{"_id": account.ID}, bson.M{
		"$set": bson.M{
			"accessToken": tokenResp.AccessToken,
			"tokenExpiry": expiry,
			"updatedAt":   time.Now(),
		},
	})
	return err
}
