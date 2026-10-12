package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"socialmedia/internal/models"
)

// AccountStats are the profile counters Instagram reports for an account.
type AccountStats struct {
	FollowersCount int `json:"followersCount"`
	FollowsCount   int `json:"followsCount"`
	MediaCount     int `json:"mediaCount"`
}

// igGet issues a GET against the Instagram Graph API and decodes the JSON body
// into out, turning an API error object into a Go error.
func igGet(ctx context.Context, path string, params url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/%s?%s", igGraphAPI, path, params.Encode()), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var apiErr struct {
		Error *igError `json:"error"`
	}
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != nil {
		return apiErr.Error
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("IG returned HTTP %d", resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

// FetchAccountStats reads the follower, following and media counts of an
// account. It needs only instagram_business_basic.
func (s *InstagramService) FetchAccountStats(ctx context.Context, account *models.SocialAccount) (*AccountStats, error) {
	if account.AccessToken == "" || account.IGUserID == "" {
		return nil, fmt.Errorf("account missing access token or user ID")
	}
	var out struct {
		FollowersCount int `json:"followers_count"`
		FollowsCount   int `json:"follows_count"`
		MediaCount     int `json:"media_count"`
	}
	if err := igGet(ctx, account.IGUserID, url.Values{
		"fields":       {"followers_count,follows_count,media_count"},
		"access_token": {account.AccessToken},
	}, &out); err != nil {
		return nil, err
	}
	return &AccountStats{FollowersCount: out.FollowersCount, FollowsCount: out.FollowsCount, MediaCount: out.MediaCount}, nil
}

// insightsResponse is the shape of an /insights reply. Lifetime media metrics
// carry their number in values[0]; total_value metrics in total_value.
type insightsResponse struct {
	Data []struct {
		Name   string `json:"name"`
		Values []struct {
			Value json.RawMessage `json:"value"`
		} `json:"values"`
		TotalValue *struct {
			Value int `json:"value"`
		} `json:"total_value"`
	} `json:"data"`
}

func (r insightsResponse) values() map[string]int {
	out := map[string]int{}
	for _, d := range r.Data {
		if d.TotalValue != nil {
			out[d.Name] = d.TotalValue.Value
			continue
		}
		if len(d.Values) > 0 {
			var n int
			if json.Unmarshal(d.Values[0].Value, &n) == nil {
				out[d.Name] = n
			}
		}
	}
	return out
}

// Instagram refuses the whole request when one metric does not apply to the
// media type, so each type asks for its own set, and a refused set falls back
// to the metrics every feed post and reel supports.
var (
	igFeedMetrics  = []string{"reach", "views", "saved", "shares", "likes", "comments", "total_interactions", "follows", "profile_visits"}
	igReelMetrics  = []string{"reach", "views", "saved", "shares", "likes", "comments", "total_interactions"}
	igStoryMetrics = []string{"reach", "views", "shares", "total_interactions", "follows", "profile_visits"}
	igBaseMetrics  = []string{"reach", "saved", "shares", "likes", "comments"}
)

// FetchMediaInsights reads the Insights of one published media object. It
// needs instagram_business_manage_insights; without it the returned error says
// so and the account has to be reconnected.
func (s *InstagramService) FetchMediaInsights(ctx context.Context, account *models.SocialAccount, mediaID string) (*models.PostInsights, error) {
	var media struct {
		MediaProductType string `json:"media_product_type"`
		Permalink        string `json:"permalink"`
		LikeCount        int    `json:"like_count"`
		CommentsCount    int    `json:"comments_count"`
	}
	if err := igGet(ctx, mediaID, url.Values{
		"fields":       {"media_product_type,permalink,like_count,comments_count"},
		"access_token": {account.AccessToken},
	}, &media); err != nil {
		return nil, err
	}

	metrics := igFeedMetrics
	switch media.MediaProductType {
	case "REELS":
		metrics = igReelMetrics
	case "STORY":
		metrics = igStoryMetrics
	}

	var resp insightsResponse
	err := igGet(ctx, mediaID+"/insights", url.Values{
		"metric":       {strings.Join(metrics, ",")},
		"access_token": {account.AccessToken},
	}, &resp)
	if err != nil && media.MediaProductType != "STORY" {
		resp = insightsResponse{}
		err = igGet(ctx, mediaID+"/insights", url.Values{
			"metric":       {strings.Join(igBaseMetrics, ",")},
			"access_token": {account.AccessToken},
		}, &resp)
	}
	if err != nil {
		return nil, err
	}

	v := resp.values()
	ins := &models.PostInsights{
		MediaProductType:  media.MediaProductType,
		Permalink:         media.Permalink,
		Reach:             v["reach"],
		Views:             v["views"],
		Shares:            v["shares"],
		Saved:             v["saved"],
		Likes:             v["likes"],
		Comments:          v["comments"],
		TotalInteractions: v["total_interactions"],
		Follows:           v["follows"],
		ProfileVisits:     v["profile_visits"],
		FetchedAt:         time.Now(),
	}
	if ins.Likes == 0 {
		ins.Likes = media.LikeCount
	}
	if ins.Comments == 0 {
		ins.Comments = media.CommentsCount
	}
	return ins, nil
}

// accountInsightMetrics are the account-level totals FetchAccountInsights
// reads. They are requested one at a time so a metric Instagram refuses for
// this account does not cost the others.
var accountInsightMetrics = []string{"reach", "views", "profile_views", "accounts_engaged", "total_interactions", "follows_and_unfollows", "website_clicks"}

// FetchAccountInsights returns account-level totals between since and until
// (Instagram allows at most 30 days per request). Metrics Instagram refuses
// are reported in the errors map instead.
func (s *InstagramService) FetchAccountInsights(ctx context.Context, account *models.SocialAccount, since, until time.Time) (map[string]int, map[string]string) {
	totals := map[string]int{}
	errs := map[string]string{}
	for _, metric := range accountInsightMetrics {
		var resp insightsResponse
		err := igGet(ctx, account.IGUserID+"/insights", url.Values{
			"metric":       {metric},
			"period":       {"day"},
			"metric_type":  {"total_value"},
			"since":        {fmt.Sprintf("%d", since.Unix())},
			"until":        {fmt.Sprintf("%d", until.Unix())},
			"access_token": {account.AccessToken},
		}, &resp)
		if err != nil {
			errs[metric] = err.Error()
			continue
		}
		for name, value := range resp.values() {
			totals[name] = value
		}
	}
	return totals, errs
}
