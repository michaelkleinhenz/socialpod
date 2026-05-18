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

const linkedInAPIv2 = "https://api.linkedin.com/v2"

type LinkedInService struct {
	DB        *database.MongoDB
	UploadDir string
}

func (s *LinkedInService) Post(ctx context.Context, content string, imageURLs []string, accountID string) (string, error) {
	account, err := s.getAccount(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("no active LinkedIn account: %w", err)
	}

	shareContent := map[string]interface{}{
		"shareCommentary":    map[string]string{"text": content},
		"shareMediaCategory": "NONE",
	}

	if len(imageURLs) > 0 {
		var mediaItems []map[string]interface{}
		for _, imgURL := range imageURLs {
			assetURN, uploadErr := s.uploadImage(ctx, account, imgURL)
			if uploadErr != nil {
				log.Printf("LinkedIn image upload failed: %v", uploadErr)
				continue
			}
			mediaItems = append(mediaItems, map[string]interface{}{
				"status":      "READY",
				"description": map[string]string{"text": ""},
				"media":       assetURN,
				"title":       map[string]string{"text": ""},
			})
		}
		if len(mediaItems) > 0 {
			shareContent["shareMediaCategory"] = "IMAGE"
			shareContent["media"] = mediaItems
		}
	}

	post := map[string]interface{}{
		"author":         account.LinkedInPersonURN,
		"lifecycleState": "PUBLISHED",
		"specificContent": map[string]interface{}{
			"com.linkedin.ugc.ShareContent": shareContent,
		},
		"visibility": map[string]interface{}{
			"com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC",
		},
	}

	postJSON, _ := json.Marshal(post)
	req, err := http.NewRequestWithContext(ctx, "POST", linkedInAPIv2+"/ugcPosts", bytes.NewReader(postJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("LinkedIn post: status=%d body=%s", resp.StatusCode, string(body))

	if resp.StatusCode != 201 {
		return "", fmt.Errorf("LinkedIn post failed (%d): %s", resp.StatusCode, string(body))
	}

	postURN := resp.Header.Get("X-RestLi-Id")
	if postURN == "" {
		var result struct {
			ID string `json:"id"`
		}
		json.Unmarshal(body, &result)
		postURN = result.ID
	}
	return postURN, nil
}

func (s *LinkedInService) uploadImage(ctx context.Context, account *models.SocialAccount, imageURL string) (string, error) {
	registerBody := map[string]interface{}{
		"registerUploadRequest": map[string]interface{}{
			"recipes": []string{"urn:li:digitalmediaRecipe:feedshare-image"},
			"owner":   account.LinkedInPersonURN,
			"serviceRelationships": []map[string]interface{}{
				{"relationshipType": "OWNER", "identifier": "urn:li:userGeneratedContent"},
			},
		},
	}

	regJSON, _ := json.Marshal(registerBody)
	req, err := http.NewRequestWithContext(ctx, "POST", linkedInAPIv2+"/assets?action=registerUpload", bytes.NewReader(regJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	regBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("LinkedIn register upload failed (%d): %s", resp.StatusCode, string(regBody))
	}

	var regResult struct {
		Value struct {
			Asset          string `json:"asset"`
			UploadMechanism struct {
				Request struct {
					UploadURL string `json:"uploadUrl"`
				} `json:"com.linkedin.digitalmedia.uploading.MediaUploadHttpRequest"`
			} `json:"uploadMechanism"`
		} `json:"value"`
	}
	json.Unmarshal(regBody, &regResult)

	uploadURL := regResult.Value.UploadMechanism.Request.UploadURL
	assetURN := regResult.Value.Asset
	if uploadURL == "" || assetURN == "" {
		return "", fmt.Errorf("LinkedIn register upload returned empty uploadUrl or asset")
	}

	// Read image data from disk or MongoDB.
	filename := strings.TrimPrefix(imageURL, "/api/uploads/")
	var fileData []byte
	diskPath := filepath.Join(s.UploadDir, filename)
	if d, readErr := os.ReadFile(diskPath); readErr == nil {
		fileData = d
	} else {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var upload models.Upload
		if dbErr := s.DB.Uploads().FindOne(dbCtx, bson.M{"filename": filename}).Decode(&upload); dbErr != nil {
			return "", fmt.Errorf("image not found: %s", filename)
		}
		fileData = upload.Data
	}

	uploadReq, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, bytes.NewReader(fileData))
	if err != nil {
		return "", err
	}
	uploadReq.Header.Set("Authorization", "Bearer "+account.AccessToken)

	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		return "", err
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != 201 && uploadResp.StatusCode != 200 {
		upBody, _ := io.ReadAll(uploadResp.Body)
		return "", fmt.Errorf("LinkedIn image upload failed (%d): %s", uploadResp.StatusCode, string(upBody))
	}
	return assetURN, nil
}

// FetchProfile fetches the LinkedIn user's display name and avatar, and sets
// account.LinkedInPersonURN as a side effect.
func (s *LinkedInService) FetchProfile(account *models.SocialAccount) (displayName, avatarURL string, err error) {
	req, err := http.NewRequest("GET",
		linkedInAPIv2+"/me?projection=(id,localizedFirstName,localizedLastName,profilePicture(displayImage~:playableStreams))", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("LinkedIn /me failed (%d): %s", resp.StatusCode, string(body))
	}

	var profile struct {
		ID                 string `json:"id"`
		LocalizedFirstName string `json:"localizedFirstName"`
		LocalizedLastName  string `json:"localizedLastName"`
		ProfilePicture     struct {
			DisplayImage struct {
				Elements []struct {
					Identifiers []struct {
						Identifier string `json:"identifier"`
					} `json:"identifiers"`
				} `json:"elements"`
			} `json:"displayImage~"`
		} `json:"profilePicture"`
	}
	json.Unmarshal(body, &profile)

	if profile.ID != "" && account.LinkedInPersonURN == "" {
		account.LinkedInPersonURN = "urn:li:person:" + profile.ID
	}

	name := strings.TrimSpace(profile.LocalizedFirstName + " " + profile.LocalizedLastName)

	avatar := ""
	elements := profile.ProfilePicture.DisplayImage.Elements
	if len(elements) > 0 {
		last := elements[len(elements)-1]
		if len(last.Identifiers) > 0 {
			avatar = last.Identifiers[0].Identifier
		}
	}

	return name, avatar, nil
}

func (s *LinkedInService) getAccount(ctx context.Context, accountID string) (*models.SocialAccount, error) {
	filter := bson.M{"platform": models.PlatformLinkedIn, "isActive": true}
	if accountID != "" {
		id, err := primitive.ObjectIDFromHex(accountID)
		if err != nil {
			return nil, fmt.Errorf("invalid linkedin account ID: %s", accountID)
		}
		filter["_id"] = id
	}
	var account models.SocialAccount
	err := s.DB.SocialAccounts().FindOne(ctx, filter).Decode(&account)
	return &account, err
}
