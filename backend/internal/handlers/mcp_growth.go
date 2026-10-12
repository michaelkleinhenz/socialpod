package handlers

import (
	"context"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func intArg(args map[string]any, key string, def int) int {
	if v, ok := args[key].(float64); ok && v > 0 {
		return int(v)
	}
	return def
}

func growthToolDefinitions() []mcpTool {
	return []mcpTool{
		{
			Name: "get_post_insights",
			Description: "Instagram Insights of published posts: reach, views, shares (sends), saves, likes, comments, follows and profile visits, " +
				"with sends and saves per reach and an adCandidate flag for the reels and carousels in the top 20% by shares plus saves over the last 30 days. " +
				"Pass postId for one post, or days for every Instagram post published in that window. Numbers are refreshed hourly in the background; refresh=true reads them from Instagram now.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"postId":  map[string]any{"type": "string", "description": "One post's ID"},
					"days":    map[string]any{"type": "number", "description": "Look-back window in days when no postId is given (default 7, max 90)"},
					"refresh": map[string]any{"type": "boolean", "description": "Read fresh numbers from Instagram before answering (slower)"},
				},
			},
		},
		{
			Name: "get_account_insights",
			Description: "Instagram account overview for the weekly review: current followers, the follower goal tracker (start, target, required and actual net followers per week, halfway review status), " +
				"the weekly report (top 3 posts, weakest format, sends and saves per reach by format: reel, carousel, image) and account totals such as reach and profile views for the same range. " +
				"The range defaults to the last 7 days; pass start and end (RFC3339) for a specific week.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"start":     map[string]any{"type": "string", "description": "Report start (RFC3339)"},
					"end":       map[string]any{"type": "string", "description": "Report end (RFC3339); defaults to start + 7 days"},
					"accountId": map[string]any{"type": "string", "description": "Instagram account ID when the team has several (see list_accounts)"},
				},
			},
		},
	}
}

func (h *MCPHandler) growth() *GrowthHandler {
	return &GrowthHandler{DB: h.DB, Scheduler: h.Scheduler, UploadDir: h.UploadDir}
}

func (h *MCPHandler) toolGetPostInsights(c *gin.Context, args map[string]any) (any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	now := time.Now()
	refresh := boolArg(args, "refresh") && h.Scheduler != nil

	if id := strArg(args, "postId"); id != "" {
		postID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return map[string]string{"error": "Invalid post ID"}, true
		}
		filter := mcpScopeFilter(c)
		filter["_id"] = postID
		var post models.Post
		if err := h.DB.Posts().FindOne(ctx, filter).Decode(&post); err != nil {
			return map[string]string{"error": "Post not found"}, true
		}
		if instagramResult(post) == nil {
			return map[string]string{"error": "This post has not been published to Instagram"}, true
		}
		if refresh {
			if err := h.Scheduler.RefreshPostInsights(ctx, post); err == nil {
				h.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
			}
		}
		perf, _ := performanceOf(post)
		// The ad flag ranks the post against the last 30 days.
		recent, err := h.growth().loadInstagramPosts(ctx, c, now.Add(-adCandidateWindow))
		if err == nil {
			ids, _ := adCandidateIDs(recent, now)
			perf.AdCandidate = ids[perf.PostID]
		}
		return perf, false
	}

	days := intArg(args, "days", 7)
	if days > 90 {
		days = 90
	}
	since := now.AddDate(0, 0, -days)
	loadFrom := since
	if s := now.Add(-adCandidateWindow); s.Before(loadFrom) {
		loadFrom = s
	}
	posts, err := h.growth().loadInstagramPosts(ctx, c, loadFrom)
	if err != nil {
		return map[string]string{"error": "Failed to load posts"}, true
	}
	if refresh {
		for _, p := range posts {
			if r := instagramResult(p); r != nil && !r.PostedAt.Before(since) && p.PostType != models.PostTypeStory {
				h.Scheduler.RefreshPostInsights(ctx, p)
			}
		}
		posts, _ = h.growth().loadInstagramPosts(ctx, c, loadFrom)
	}
	ids, _ := adCandidateIDs(posts, now)
	out := []PostPerformance{}
	for _, p := range posts {
		perf, ok := performanceOf(p)
		if !ok || perf.PostedAt.Before(since) {
			continue
		}
		perf.AdCandidate = ids[perf.PostID]
		out = append(out, perf)
	}
	return map[string]any{"since": since, "posts": out}, false
}

func (h *MCPHandler) toolGetAccountInsights(c *gin.Context, args map[string]any) (any, bool) {
	teamID := teamIDFrom(c)
	if teamID == nil {
		return map[string]string{"error": "Instagram growth tracking needs a team"}, true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	g := h.growth()
	now := time.Now()

	start := now.AddDate(0, 0, -7)
	end := now
	if s, err := time.Parse(time.RFC3339, strArg(args, "start")); err == nil {
		start = s
		end = s.AddDate(0, 0, 7)
	}
	if e, err := time.Parse(time.RFC3339, strArg(args, "end")); err == nil && e.After(start) {
		end = e
	}

	accounts, err := g.teamInstagramAccounts(ctx, *teamID)
	if err != nil {
		return map[string]string{"error": "Failed to load accounts"}, true
	}
	account := pickAccount(accounts, strArg(args, "accountId"))
	if account == nil {
		return map[string]string{"error": "The team has no active Instagram account"}, true
	}
	settings, _ := g.loadGrowthSettings(ctx, *teamID)

	snaps, err := g.loadSnapshots(ctx, account.ID, "")
	if err != nil {
		return map[string]string{"error": "Failed to load follower history"}, true
	}
	loadFrom := start
	if s := now.Add(-adCandidateWindow); s.Before(loadFrom) {
		loadFrom = s
	}
	posts, err := g.loadInstagramPosts(ctx, c, loadFrom)
	if err != nil {
		return map[string]string{"error": "Failed to load posts"}, true
	}
	ids, candidates := adCandidateIDs(posts, now)

	out := map[string]any{
		"account":      summarizeAccounts([]models.SocialAccount{*account})[0],
		"report":       buildWeeklyReport(posts, start, end, snaps, ids),
		"adCandidates": candidates,
	}
	if len(snaps) > 0 {
		out["currentFollowers"] = snaps[len(snaps)-1].FollowersCount
		out["followersAsOf"] = snaps[len(snaps)-1].Date
	}
	if settings.InstagramGoal != nil {
		out["goal"] = computeGoalProgress(*settings.InstagramGoal, snaps, now.UTC().Format(dateLayout))
	}

	// Account totals come straight from Instagram; a missing permission or a
	// range Instagram does not cover only costs this part.
	if h.Scheduler != nil {
		var full models.SocialAccount
		if h.DB.SocialAccounts().FindOne(ctx, bson.M{"_id": account.ID}).Decode(&full) == nil {
			apiEnd := end
			if apiEnd.After(now) {
				apiEnd = now
			}
			totals, errs := h.Scheduler.Instagram.FetchAccountInsights(ctx, &full, start, apiEnd)
			out["accountTotals"] = totals
			if len(errs) > 0 {
				out["accountTotalsErrors"] = errs
			}
		}
	}
	return out, false
}
