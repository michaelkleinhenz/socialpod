package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"
	"socialmedia/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GrowthHandler serves the Instagram growth features: team growth settings,
// the follower goal tracker, the weekly report and Insights refreshes.
type GrowthHandler struct {
	DB        *database.MongoDB
	Scheduler *services.Scheduler
	UploadDir string
}

// adCandidateWindow is the stretch of recent posts the top-20% ad rule ranks.
const adCandidateWindow = 30 * 24 * time.Hour

// teamIDFrom returns the caller's team, or nil when they have none.
func teamIDFrom(c *gin.Context) *primitive.ObjectID {
	v, ok := c.Get("teamId")
	if !ok {
		return nil
	}
	s, _ := v.(string)
	if s == "" {
		return nil
	}
	id, err := primitive.ObjectIDFromHex(s)
	if err != nil {
		return nil
	}
	return &id
}

// defaultGrowthSettings fills what a team has not configured yet.
func defaultGrowthSettings(g *models.TeamGrowthSettings) models.TeamGrowthSettings {
	out := models.TeamGrowthSettings{}
	if g != nil {
		out = *g
	}
	if out.WeeklyPlan == nil {
		out.WeeklyPlan = []models.WeeklyPlanSlot{}
	}
	if out.MaxSingleImagesPerWeek <= 0 {
		out.MaxSingleImagesPerWeek = 2
	}
	radar := models.RadarSettings{}
	if out.Radar != nil {
		radar = *out.Radar
	}
	if radar.Title == "" {
		radar.Title = "Radar der Woche"
	}
	if radar.BadgeQuestion == "" {
		radar.BadgeQuestion = "Kommt's auf Deutsch?"
	}
	if radar.AccentColor == "" {
		radar.AccentColor = "#6366f1"
	}
	out.Radar = &radar
	return out
}

func (h *GrowthHandler) loadGrowthSettings(ctx context.Context, teamID primitive.ObjectID) (models.TeamGrowthSettings, error) {
	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		return models.TeamGrowthSettings{}, err
	}
	return defaultGrowthSettings(team.Growth), nil
}

// GetSettings returns the team's growth settings (any team member).
// GET /api/growth/settings
func (h *GrowthHandler) GetSettings(c *gin.Context) {
	teamID := teamIDFrom(c)
	if teamID == nil {
		c.JSON(http.StatusOK, defaultGrowthSettings(nil))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	settings, err := h.loadGrowthSettings(ctx, *teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}
	c.JSON(http.StatusOK, settings)
}

var (
	hhmmRe     = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

func validSlotFormat(f string) bool {
	switch f {
	case models.SlotFormatReel, models.SlotFormatCarousel, models.SlotFormatImage, models.SlotFormatStory, models.SlotFormatAny:
		return true
	}
	return false
}

// validateGrowthSettings checks a settings document a team admin submitted.
func validateGrowthSettings(g *models.TeamGrowthSettings) error {
	if goal := g.InstagramGoal; goal != nil {
		if goal.Target <= 0 {
			return fmt.Errorf("the follower target must be positive")
		}
		s, err1 := time.Parse(dateLayout, goal.StartDate)
		e, err2 := time.Parse(dateLayout, goal.EndDate)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("goal dates must be YYYY-MM-DD")
		}
		if !e.After(s) {
			return fmt.Errorf("the goal must end after it starts")
		}
		if goal.HalfwayDate != "" {
			if _, err := time.Parse(dateLayout, goal.HalfwayDate); err != nil {
				return fmt.Errorf("the halfway date must be YYYY-MM-DD")
			}
		}
		if goal.HalfwayThreshold < 0 || goal.HalfwayThreshold >= 1 {
			return fmt.Errorf("the halfway threshold must be between 0 and 1")
		}
		if goal.StartFollowers != nil && *goal.StartFollowers < 0 {
			return fmt.Errorf("the starting follower count cannot be negative")
		}
	}
	if len(g.WeeklyPlan) > 50 {
		return fmt.Errorf("the weekly plan holds at most 50 slots")
	}
	for i := range g.WeeklyPlan {
		slot := &g.WeeklyPlan[i]
		if slot.Weekday < 0 || slot.Weekday > 6 {
			return fmt.Errorf("slot %d: weekday must be 0 (Monday) to 6 (Sunday)", i+1)
		}
		if slot.Time != "" && !hhmmRe.MatchString(slot.Time) {
			return fmt.Errorf("slot %d: time must be HH:MM", i+1)
		}
		if !validSlotFormat(slot.Format) {
			return fmt.Errorf("slot %d: unknown format %q", i+1, slot.Format)
		}
		if slot.ID == "" {
			slot.ID = primitive.NewObjectID().Hex()
		}
		slot.Label = strings.TrimSpace(slot.Label)
	}
	if g.MaxSingleImagesPerWeek < 0 {
		return fmt.Errorf("the single-image limit cannot be negative")
	}
	if r := g.Radar; r != nil {
		if r.AccentColor != "" && !hexColorRe.MatchString(r.AccentColor) {
			return fmt.Errorf("the accent colour must be a hex colour like #6366f1")
		}
		for _, u := range []string{r.CoverImageURL, r.ClosingImageURL} {
			if u != "" && uploadFilename(u) == "" {
				return fmt.Errorf("radar slide images must be uploaded images")
			}
		}
	}
	return nil
}

// UpdateSettings replaces the team's growth settings (team admin).
// PUT /api/team/growth
func (h *GrowthHandler) UpdateSettings(c *gin.Context) {
	teamID := teamIDFrom(c)
	if teamID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No team associated with this account"})
		return
	}
	var input models.TeamGrowthSettings
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateGrowthSettings(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}
	// Slide images the update replaces are freed once nothing points at them.
	var dropped []string
	if old := team.Growth; old != nil && old.Radar != nil {
		newRadar := input.Radar
		if newRadar == nil {
			newRadar = &models.RadarSettings{}
		}
		if old.Radar.CoverImageURL != "" && old.Radar.CoverImageURL != newRadar.CoverImageURL {
			dropped = append(dropped, old.Radar.CoverImageURL)
		}
		if old.Radar.ClosingImageURL != "" && old.Radar.ClosingImageURL != newRadar.ClosingImageURL {
			dropped = append(dropped, old.Radar.ClosingImageURL)
		}
	}

	if _, err := h.DB.Teams().UpdateOne(ctx, bson.M{"_id": teamID}, bson.M{"$set": bson.M{
		"growth":    input,
		"updatedAt": time.Now(),
	}}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
		return
	}
	cleanupUploads(h.DB, h.UploadDir, dropped)
	c.JSON(http.StatusOK, defaultGrowthSettings(&input))
}

// teamInstagramAccounts lists the team's active Instagram accounts.
func (h *GrowthHandler) teamInstagramAccounts(ctx context.Context, teamID primitive.ObjectID) ([]models.SocialAccount, error) {
	cursor, err := h.DB.SocialAccounts().Find(ctx, bson.M{
		"platform": models.PlatformInstagram,
		"isActive": true,
		"teamId":   teamID,
	}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var accounts []models.SocialAccount
	err = cursor.All(ctx, &accounts)
	return accounts, err
}

// pickAccount resolves the ?accountId= query against the team's Instagram
// accounts, defaulting to the first one.
func pickAccount(accounts []models.SocialAccount, requested string) *models.SocialAccount {
	for i := range accounts {
		if accounts[i].ID.Hex() == requested {
			return &accounts[i]
		}
	}
	if len(accounts) > 0 {
		return &accounts[0]
	}
	return nil
}

type accountSummary struct {
	ID          string `json:"id"`
	AccountName string `json:"accountName"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
}

func summarizeAccounts(accounts []models.SocialAccount) []accountSummary {
	out := make([]accountSummary, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, accountSummary{ID: a.ID.Hex(), AccountName: a.AccountName, AvatarURL: a.AvatarURL})
	}
	return out
}

func (h *GrowthHandler) loadSnapshots(ctx context.Context, accountID primitive.ObjectID, since string) ([]models.FollowerSnapshot, error) {
	filter := bson.M{"accountId": accountID}
	if since != "" {
		filter["date"] = bson.M{"$gte": since}
	}
	cursor, err := h.DB.FollowerSnapshots().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var snaps []models.FollowerSnapshot
	err = cursor.All(ctx, &snaps)
	return snaps, err
}

// instagramPostsFilter scopes posts to the caller and to those published to
// Instagram since the given time. postedAt lives on the result, so this
// filters on the result's own timestamp.
func instagramPostsFilter(c *gin.Context, since time.Time) bson.M {
	filter := postFilter(c)
	filter["results"] = bson.M{"$elemMatch": bson.M{
		"platform": models.PlatformInstagram,
		"success":  true,
		"postedAt": bson.M{"$gte": since},
	}}
	return filter
}

func (h *GrowthHandler) loadInstagramPosts(ctx context.Context, c *gin.Context, since time.Time) ([]models.Post, error) {
	cursor, err := h.DB.Posts().Find(ctx, instagramPostsFilter(c, since))
	if err != nil {
		return nil, err
	}
	var posts []models.Post
	err = cursor.All(ctx, &posts)
	return posts, err
}

// adCandidateIDs ranks the recent reels and carousels and returns the IDs of
// those in the top 20%.
func adCandidateIDs(posts []models.Post, now time.Time) (map[string]bool, []PostPerformance) {
	var perfs []PostPerformance
	for _, post := range posts {
		if p, ok := performanceOf(post); ok && !p.PostedAt.Before(now.Add(-adCandidateWindow)) && p.Format != formatStory {
			perfs = append(perfs, p)
		}
	}
	markAdCandidates(perfs)
	ids := map[string]bool{}
	var candidates []PostPerformance
	for _, p := range perfs {
		if p.AdCandidate {
			ids[p.PostID] = true
			candidates = append(candidates, p)
		}
	}
	sort.SliceStable(candidates, func(a, b int) bool { return candidates[a].Score > candidates[b].Score })
	return ids, candidates
}

// Overview returns everything the Growth page shows: the goal tracker, the
// weekly report for the requested week and the current ad candidates.
// GET /api/growth/instagram?accountId=&start=&end=
//
// start and end bound the report week (RFC3339); the client sends its own
// local Monday so the week matches the calendar the team sees. Without them
// the report covers the last seven days.
func (h *GrowthHandler) Overview(c *gin.Context) {
	teamID := teamIDFrom(c)
	if teamID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Growth tracking needs a team"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	settings, err := h.loadGrowthSettings(ctx, *teamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}
	accounts, err := h.teamInstagramAccounts(ctx, *teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load accounts"})
		return
	}

	now := time.Now()
	start, end := reportRange(c, now)

	resp := gin.H{
		"accounts": summarizeAccounts(accounts),
		"settings": settings,
	}

	account := pickAccount(accounts, c.Query("accountId"))
	var snaps []models.FollowerSnapshot
	if account != nil {
		resp["accountId"] = account.ID.Hex()
		snaps, err = h.loadSnapshots(ctx, account.ID, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load follower history"})
			return
		}
		if settings.InstagramGoal != nil {
			resp["goal"] = computeGoalProgress(*settings.InstagramGoal, snaps, now.UTC().Format(dateLayout))
		}
		if len(snaps) > 0 {
			resp["latestSnapshot"] = snaps[len(snaps)-1]
		}
	}

	since := start
	if adSince := now.Add(-adCandidateWindow); adSince.Before(since) {
		since = adSince
	}
	posts, err := h.loadInstagramPosts(ctx, c, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load posts"})
		return
	}
	candidateIDs, candidates := adCandidateIDs(posts, now)
	if candidates == nil {
		candidates = []PostPerformance{}
	}
	resp["report"] = buildWeeklyReport(posts, start, end, snaps, candidateIDs)
	resp["adCandidates"] = candidates
	c.JSON(http.StatusOK, resp)
}

// reportRange reads the ?start=&end= report week, defaulting to the last
// seven days.
func reportRange(c *gin.Context, now time.Time) (time.Time, time.Time) {
	end := now
	start := now.AddDate(0, 0, -7)
	if s, err := time.Parse(time.RFC3339, c.Query("start")); err == nil {
		start = s
		end = s.AddDate(0, 0, 7)
	}
	if e, err := time.Parse(time.RFC3339, c.Query("end")); err == nil && e.After(start) {
		end = e
	}
	return start, end
}

// RefreshInsights re-reads today's follower count and the Insights of the
// team's Instagram posts from the last 30 days.
// POST /api/growth/refresh
func (h *GrowthHandler) RefreshInsights(c *gin.Context) {
	teamID := teamIDFrom(c)
	if teamID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Growth tracking needs a team"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	accounts, err := h.teamInstagramAccounts(ctx, *teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load accounts"})
		return
	}
	var errs []string
	for i := range accounts {
		if err := services.RecordFollowerSnapshot(ctx, h.DB, h.Scheduler.Instagram, &accounts[i]); err != nil {
			errs = append(errs, fmt.Sprintf("@%s followers: %v", accounts[i].AccountName, err))
		}
	}

	posts, err := h.loadInstagramPosts(ctx, c, time.Now().Add(-adCandidateWindow))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load posts"})
		return
	}
	refreshed := 0
	var firstInsightsErr string
	for _, post := range posts {
		if post.PostType == models.PostTypeStory {
			continue
		}
		if err := h.Scheduler.RefreshPostInsights(ctx, post); err != nil {
			if firstInsightsErr == "" {
				firstInsightsErr = err.Error()
			}
			continue
		}
		refreshed++
	}
	if firstInsightsErr != "" {
		errs = append(errs, "Insights: "+firstInsightsErr)
	}
	log.Printf("Growth refresh for team %s: %d posts, %d errors", teamID.Hex(), refreshed, len(errs))
	c.JSON(http.StatusOK, gin.H{"refreshed": refreshed, "posts": len(posts), "errors": errs})
}

// RefreshPostInsights re-reads one post's Instagram Insights.
// POST /api/posts/:id/insights
func (h *GrowthHandler) RefreshPostInsights(c *gin.Context) {
	postID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post ID"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := postFilter(c)
	filter["_id"] = postID
	var post models.Post
	if err := h.DB.Posts().FindOne(ctx, filter).Decode(&post); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}
	if instagramResult(post) == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This post has not been published to Instagram"})
		return
	}
	refreshErr := h.Scheduler.RefreshPostInsights(ctx, post)
	h.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	if refreshErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Instagram did not return Insights: " + refreshErr.Error(), "post": post})
		return
	}
	c.JSON(http.StatusOK, post)
}

// instagramPerformanceSummary describes the last 30 days of Instagram
// performance in plain text, for the dashboard's AI insights. It returns ""
// when there is nothing to report.
func instagramPerformanceSummary(ctx context.Context, db *database.MongoDB, c *gin.Context) string {
	now := time.Now()
	h := &GrowthHandler{DB: db}
	posts, err := h.loadInstagramPosts(ctx, c, now.Add(-adCandidateWindow))
	if err != nil {
		return ""
	}
	candidateIDs, _ := adCandidateIDs(posts, now)
	rep := buildWeeklyReport(posts, now.Add(-adCandidateWindow), now, nil, candidateIDs)

	var b strings.Builder
	if len(rep.Posts) > 0 {
		fmt.Fprintf(&b, "Instagram performance, last 30 days (%d feed posts and reels, %d stories; %d without Insights yet):\n", len(rep.Posts), rep.Stories, rep.MissingInsights)
		for _, f := range rep.Formats {
			if f.Posts == 0 {
				continue
			}
			fmt.Fprintf(&b, "- %s: %d posts, avg reach %d, sends/reach %.2f%%, saves/reach %.2f%%, follows %d\n",
				f.Format, f.Posts, f.AvgReach, f.SendsPerReach*100, f.SavesPerReach*100, f.Follows)
		}
		if rep.WeakestFormat != "" {
			fmt.Fprintf(&b, "Weakest format by sends+saves per reach: %s\n", rep.WeakestFormat)
		}
		if len(rep.TopPosts) > 0 {
			b.WriteString("Best posts by sends+saves:\n")
			for _, p := range rep.TopPosts {
				fmt.Fprintf(&b, "- [%s, %s] reach %d, sends %d, saves %d: %q\n", p.Format, p.PostedAt.Format("Jan 2"), p.Reach, p.Shares, p.Saved, p.Caption)
			}
		}
	}

	if teamID := teamIDFrom(c); teamID != nil {
		accounts, err := h.teamInstagramAccounts(ctx, *teamID)
		if err == nil && len(accounts) > 0 {
			snaps, _ := h.loadSnapshots(ctx, accounts[0].ID, now.AddDate(0, 0, -30).Format(dateLayout))
			if len(snaps) > 0 {
				first, last := snaps[0], snaps[len(snaps)-1]
				fmt.Fprintf(&b, "Instagram followers: %d on %s, %d on %s (net %+d).\n",
					first.FollowersCount, first.Date, last.FollowersCount, last.Date, last.FollowersCount-first.FollowersCount)
			}
			var team models.Team
			if db.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team) == nil && team.Growth != nil && team.Growth.InstagramGoal != nil {
				all, _ := h.loadSnapshots(ctx, accounts[0].ID, "")
				gp := computeGoalProgress(*team.Growth.InstagramGoal, all, now.UTC().Format(dateLayout))
				fmt.Fprintf(&b, "Follower goal: %d by %s", gp.Goal.Target, gp.Goal.EndDate)
				if gp.StartFollowers != nil {
					fmt.Fprintf(&b, ", starting from %d, needing %.0f net followers per week", *gp.StartFollowers, gp.RequiredPerWeek)
				}
				if gp.Progress != nil {
					fmt.Fprintf(&b, "; %.0f%% of the way so far", *gp.Progress*100)
				}
				if gp.ExpectedToday != nil {
					fmt.Fprintf(&b, " (plan expects %d followers by today)", *gp.ExpectedToday)
				}
				b.WriteString(".\n")
			}
		}
	}
	return b.String()
}
