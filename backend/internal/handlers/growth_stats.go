package handlers

import (
	"math"
	"sort"
	"strings"
	"time"

	"socialmedia/internal/models"
)

// Instagram formats the growth reports compare. A story is reported apart:
// it reaches followers only and has no share-to-reach signal worth ranking.
const (
	formatReel     = "reel"
	formatCarousel = "carousel"
	formatImage    = "image"
	formatStory    = "story"
)

var reportFormats = []string{formatReel, formatCarousel, formatImage}

// instagramFormat classifies a post the way the growth plan talks about it.
func instagramFormat(post models.Post) string {
	switch post.PostType {
	case models.PostTypeReel:
		return formatReel
	case models.PostTypeStory:
		return formatStory
	}
	if len(post.ImageURLs) > 1 {
		return formatCarousel
	}
	if len(post.ImageURLs) == 1 && isVideoURL(post.ImageURLs[0]) {
		return formatReel
	}
	return formatImage
}

func isVideoURL(u string) bool {
	l := strings.ToLower(u)
	return strings.HasSuffix(l, ".mp4") || strings.HasSuffix(l, ".mov")
}

// instagramResult returns the successful Instagram result of a post, if any.
func instagramResult(post models.Post) *models.PostResult {
	for i := range post.Results {
		r := &post.Results[i]
		if r.Platform == models.PlatformInstagram && r.Success && r.PostID != "" {
			return r
		}
	}
	return nil
}

// PostPerformance is one published Instagram post with its numbers, as the
// reports and the MCP tools present it.
type PostPerformance struct {
	PostID        string    `json:"postId"`
	Format        string    `json:"format"`
	PostedAt      time.Time `json:"postedAt"`
	Caption       string    `json:"caption"`
	ImageURL      string    `json:"imageUrl,omitempty"`
	Permalink     string    `json:"permalink,omitempty"`
	Reach         int       `json:"reach"`
	Views         int       `json:"views"`
	Shares        int       `json:"shares"`
	Saved         int       `json:"saved"`
	Likes         int       `json:"likes"`
	Comments      int       `json:"comments"`
	Follows       int       `json:"follows"`
	ProfileVisits int       `json:"profileVisits"`
	SendsPerReach float64   `json:"sendsPerReach"`
	SavesPerReach float64   `json:"savesPerReach"`
	// Score is shares plus saves: the plan's measure of what is worth
	// promoting.
	Score         int    `json:"score"`
	HasInsights   bool   `json:"hasInsights"`
	InsightsError string `json:"insightsError,omitempty"`
	AdCandidate   bool   `json:"adCandidate"`
}

func ratio(a, b int) float64 {
	if b <= 0 {
		return 0
	}
	return math.Round(float64(a)/float64(b)*10000) / 10000
}

// performanceOf builds a PostPerformance from a post that has a successful
// Instagram result; ok is false otherwise.
func performanceOf(post models.Post) (PostPerformance, bool) {
	r := instagramResult(post)
	if r == nil {
		return PostPerformance{}, false
	}
	caption := post.Content
	if post.ContentOverrides != nil && post.ContentOverrides["instagram"] != "" {
		caption = post.ContentOverrides["instagram"]
	}
	p := PostPerformance{
		PostID:   post.ID.Hex(),
		Format:   instagramFormat(post),
		PostedAt: r.PostedAt,
		Caption:  firstLine(caption, 120),
	}
	if len(post.ImageURLs) > 0 {
		p.ImageURL = post.ImageURLs[0]
	}
	if ins := r.Insights; ins != nil {
		p.Permalink = ins.Permalink
		p.Reach = ins.Reach
		p.Views = ins.Views
		p.Shares = ins.Shares
		p.Saved = ins.Saved
		p.Likes = ins.Likes
		p.Comments = ins.Comments
		p.Follows = ins.Follows
		p.ProfileVisits = ins.ProfileVisits
		p.HasInsights = !ins.FetchedAt.IsZero() && ins.Error == ""
		p.InsightsError = ins.Error
		if ins.Error != "" && ins.Reach > 0 {
			// An older successful read is still worth showing.
			p.HasInsights = true
		}
	}
	p.SendsPerReach = ratio(p.Shares, p.Reach)
	p.SavesPerReach = ratio(p.Saved, p.Reach)
	p.Score = p.Shares + p.Saved
	return p, true
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > max {
		s = string(r[:max-1]) + "…"
	}
	return s
}

// markAdCandidates flags the reels and carousels in the top 20% by shares
// plus saves — the plan's rule for what gets advertised. Posts without
// numbers, or with a score of zero, are never candidates.
func markAdCandidates(perfs []PostPerformance) {
	var eligible []int
	for i, p := range perfs {
		if (p.Format == formatReel || p.Format == formatCarousel) && p.HasInsights && p.Score > 0 {
			eligible = append(eligible, i)
		}
	}
	if len(eligible) == 0 {
		return
	}
	sort.SliceStable(eligible, func(a, b int) bool {
		pa, pb := perfs[eligible[a]], perfs[eligible[b]]
		if pa.Score != pb.Score {
			return pa.Score > pb.Score
		}
		return pa.Reach > pb.Reach
	})
	n := int(math.Ceil(float64(len(eligible)) * 0.2))
	cutoff := perfs[eligible[n-1]].Score
	for _, i := range eligible {
		// Ties with the last candidate's score qualify too.
		if perfs[i].Score >= cutoff {
			perfs[i].AdCandidate = true
		}
	}
}

// FormatStats sums one format's posts in a report.
type FormatStats struct {
	Format        string  `json:"format"`
	Posts         int     `json:"posts"`
	WithInsights  int     `json:"withInsights"`
	Reach         int     `json:"reach"`
	Shares        int     `json:"shares"`
	Saved         int     `json:"saved"`
	Follows       int     `json:"follows"`
	AvgReach      int     `json:"avgReach"`
	SendsPerReach float64 `json:"sendsPerReach"`
	SavesPerReach float64 `json:"savesPerReach"`
}

// WeeklyReport is the Monday review: the week's best posts, the weakest
// format and each format's share and save rates.
type WeeklyReport struct {
	Start           time.Time         `json:"start"`
	End             time.Time         `json:"end"`
	Posts           []PostPerformance `json:"posts"`
	TopPosts        []PostPerformance `json:"topPosts"`
	Formats         []FormatStats     `json:"formats"`
	WeakestFormat   string            `json:"weakestFormat,omitempty"`
	Stories         int               `json:"stories"`
	MissingInsights int               `json:"missingInsights"`
	FollowersStart  *int              `json:"followersStart,omitempty"`
	FollowersEnd    *int              `json:"followersEnd,omitempty"`
	NetFollowers    *int              `json:"netFollowers,omitempty"`
}

// buildWeeklyReport compiles the report for posts published in [start, end).
// candidates are the ad-candidate flags computed over a longer window, so a
// post's flag does not depend on which week is being looked at.
func buildWeeklyReport(posts []models.Post, start, end time.Time, snapshots []models.FollowerSnapshot, candidates map[string]bool) WeeklyReport {
	rep := WeeklyReport{Start: start, End: end, Posts: []PostPerformance{}, TopPosts: []PostPerformance{}}

	byFormat := map[string]*FormatStats{}
	for _, f := range reportFormats {
		byFormat[f] = &FormatStats{Format: f}
	}

	for _, post := range posts {
		p, ok := performanceOf(post)
		if !ok || p.PostedAt.Before(start) || !p.PostedAt.Before(end) {
			continue
		}
		if p.Format == formatStory {
			rep.Stories++
			continue
		}
		p.AdCandidate = candidates[p.PostID]
		rep.Posts = append(rep.Posts, p)
		fs := byFormat[p.Format]
		fs.Posts++
		if !p.HasInsights {
			rep.MissingInsights++
			continue
		}
		fs.WithInsights++
		fs.Reach += p.Reach
		fs.Shares += p.Shares
		fs.Saved += p.Saved
		fs.Follows += p.Follows
	}

	weakestRate := math.MaxFloat64
	for _, f := range reportFormats {
		fs := byFormat[f]
		if fs.WithInsights > 0 {
			fs.AvgReach = fs.Reach / fs.WithInsights
		}
		fs.SendsPerReach = ratio(fs.Shares, fs.Reach)
		fs.SavesPerReach = ratio(fs.Saved, fs.Reach)
		rep.Formats = append(rep.Formats, *fs)
		if fs.WithInsights > 0 && fs.Reach > 0 {
			rate := float64(fs.Shares+fs.Saved) / float64(fs.Reach)
			if rate < weakestRate {
				weakestRate = rate
				rep.WeakestFormat = f
			}
		}
	}
	// With a single format there is nothing to compare it against.
	compared := 0
	for _, fs := range rep.Formats {
		if fs.WithInsights > 0 && fs.Reach > 0 {
			compared++
		}
	}
	if compared < 2 {
		rep.WeakestFormat = ""
	}

	ranked := make([]PostPerformance, 0, len(rep.Posts))
	for _, p := range rep.Posts {
		if p.HasInsights {
			ranked = append(ranked, p)
		}
	}
	sort.SliceStable(ranked, func(a, b int) bool {
		if ranked[a].Score != ranked[b].Score {
			return ranked[a].Score > ranked[b].Score
		}
		return ranked[a].Reach > ranked[b].Reach
	})
	if len(ranked) > 3 {
		ranked = ranked[:3]
	}
	rep.TopPosts = ranked
	sort.SliceStable(rep.Posts, func(a, b int) bool { return rep.Posts[a].PostedAt.Before(rep.Posts[b].PostedAt) })

	if before, ok := lastSnapshotBefore(snapshots, start); ok {
		rep.FollowersStart = &before.FollowersCount
	}
	if last, ok := lastSnapshotBefore(snapshots, end); ok {
		rep.FollowersEnd = &last.FollowersCount
	}
	if rep.FollowersStart != nil && rep.FollowersEnd != nil {
		net := *rep.FollowersEnd - *rep.FollowersStart
		rep.NetFollowers = &net
	}
	return rep
}

// lastSnapshotBefore returns the latest snapshot dated before t. Snapshots
// are daily and hold the count at the end of their day, so the one for the day
// before a week starts is that week's starting count.
func lastSnapshotBefore(snapshots []models.FollowerSnapshot, t time.Time) (models.FollowerSnapshot, bool) {
	day := t.UTC().Format("2006-01-02")
	var best models.FollowerSnapshot
	found := false
	for _, s := range snapshots {
		if s.Date < day && (!found || s.Date > best.Date) {
			best = s
			found = true
		}
	}
	return best, found
}

// GoalWeek is one plan week of the follower goal.
type GoalWeek struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Net      *int   `json:"net"`
	Required int    `json:"required"`
	Current  bool   `json:"current"`
}

// GoalPoint is one day of the follower chart.
type GoalPoint struct {
	Date      string `json:"date"`
	Followers int    `json:"followers"`
}

// GoalProgress is the follower goal tracker: where the account stands against
// the target, week by week, and whether the halfway review is due.
type GoalProgress struct {
	Goal             models.InstagramGoal `json:"goal"`
	StartFollowers   *int                 `json:"startFollowers"`
	CurrentFollowers *int                 `json:"currentFollowers"`
	CurrentDate      string               `json:"currentDate,omitempty"`
	RequiredPerWeek  float64              `json:"requiredPerWeek"`
	TotalWeeks       float64              `json:"totalWeeks"`
	Progress         *float64             `json:"progress"`
	ExpectedToday    *int                 `json:"expectedToday"`
	Series           []GoalPoint          `json:"series"`
	Weeks            []GoalWeek           `json:"weeks"`
	// Halfway is "pending" before the halfway date, then "ok" or "below"
	// depending on whether the account had covered HalfwayThreshold of the
	// way by that date. HalfwayProgress is the progress at the halfway date.
	Halfway         string   `json:"halfway"`
	HalfwayDate     string   `json:"halfwayDate"`
	HalfwayProgress *float64 `json:"halfwayProgress,omitempty"`
}

const dateLayout = "2006-01-02"

// withGoalDefaults fills a goal's optional fields: the halfway review sits
// midway between start and end and asks for 40% of the way.
func withGoalDefaults(g models.InstagramGoal) models.InstagramGoal {
	if g.HalfwayThreshold <= 0 || g.HalfwayThreshold >= 1 {
		g.HalfwayThreshold = 0.4
	}
	if g.HalfwayDate == "" {
		s, err1 := time.Parse(dateLayout, g.StartDate)
		e, err2 := time.Parse(dateLayout, g.EndDate)
		if err1 == nil && err2 == nil && e.After(s) {
			g.HalfwayDate = s.Add(e.Sub(s) / 2).Format(dateLayout)
		}
	}
	return g
}

// computeGoalProgress evaluates the follower goal against the daily
// snapshots (sorted or not) as of today ("2006-01-02").
func computeGoalProgress(goal models.InstagramGoal, snapshots []models.FollowerSnapshot, today string) GoalProgress {
	goal = withGoalDefaults(goal)
	gp := GoalProgress{Goal: goal, Series: []GoalPoint{}, Weeks: []GoalWeek{}, Halfway: "pending", HalfwayDate: goal.HalfwayDate}

	sorted := append([]models.FollowerSnapshot(nil), snapshots...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].Date < sorted[b].Date })

	start, err1 := time.Parse(dateLayout, goal.StartDate)
	end, err2 := time.Parse(dateLayout, goal.EndDate)
	if err1 != nil || err2 != nil || !end.After(start) {
		return gp
	}
	gp.TotalWeeks = math.Round(end.Sub(start).Hours()/24/7*10) / 10

	// The start count is the override, else the count at the end of the day
	// before the goal began, else the first count on or after the start.
	if goal.StartFollowers != nil {
		v := *goal.StartFollowers
		gp.StartFollowers = &v
	} else if s, ok := lastSnapshotBefore(sorted, start); ok {
		v := s.FollowersCount
		gp.StartFollowers = &v
	} else {
		for _, s := range sorted {
			if s.Date >= goal.StartDate {
				v := s.FollowersCount
				gp.StartFollowers = &v
				break
			}
		}
	}

	// Chart from a week before the start to today, capped at the end date.
	chartFrom := start.AddDate(0, 0, -7).Format(dateLayout)
	for _, s := range sorted {
		if s.Date >= chartFrom && s.Date <= goal.EndDate && s.Date <= today {
			gp.Series = append(gp.Series, GoalPoint{Date: s.Date, Followers: s.FollowersCount})
		}
	}
	for i := len(sorted) - 1; i >= 0; i-- {
		if sorted[i].Date <= today {
			v := sorted[i].FollowersCount
			gp.CurrentFollowers = &v
			gp.CurrentDate = sorted[i].Date
			break
		}
	}

	if gp.StartFollowers == nil {
		return gp
	}
	startCount := *gp.StartFollowers
	remaining := goal.Target - startCount
	weeks := end.Sub(start).Hours() / 24 / 7
	gp.RequiredPerWeek = math.Round(float64(remaining)/weeks*10) / 10

	todayT, _ := time.Parse(dateLayout, today)
	if gp.CurrentFollowers != nil && remaining != 0 {
		p := math.Round(float64(*gp.CurrentFollowers-startCount)/float64(remaining)*1000) / 1000
		gp.Progress = &p
	}
	if !todayT.Before(start) {
		elapsed := math.Min(todayT.Sub(start).Hours()/24/7, weeks)
		exp := startCount + int(math.Round(gp.RequiredPerWeek*elapsed))
		gp.ExpectedToday = &exp
	}

	// Plan weeks run seven days from the start date. The net of a week is the
	// last count inside it minus the count before it.
	prev := startCount
	for ws := start; ws.Before(end); ws = ws.AddDate(0, 0, 7) {
		we := ws.AddDate(0, 0, 7)
		if we.After(end) {
			we = end
		}
		w := GoalWeek{
			Start:    ws.Format(dateLayout),
			End:      we.AddDate(0, 0, -1).Format(dateLayout),
			Required: int(math.Round(gp.RequiredPerWeek * we.Sub(ws).Hours() / 24 / 7)),
			Current:  !todayT.Before(ws) && todayT.Before(we),
		}
		if !ws.After(todayT) {
			if last, ok := lastSnapshotBefore(sorted, we); ok && last.Date >= w.Start {
				net := last.FollowersCount - prev
				w.Net = &net
				prev = last.FollowersCount
			}
		}
		gp.Weeks = append(gp.Weeks, w)
	}

	if halfway, err := time.Parse(dateLayout, goal.HalfwayDate); err == nil && !todayT.Before(halfway) && remaining != 0 {
		// The count at the end of the halfway day.
		if s, ok := lastSnapshotBefore(sorted, halfway.AddDate(0, 0, 1)); ok {
			p := math.Round(float64(s.FollowersCount-startCount)/float64(remaining)*1000) / 1000
			gp.HalfwayProgress = &p
			if p < goal.HalfwayThreshold {
				gp.Halfway = "below"
			} else {
				gp.Halfway = "ok"
			}
		}
	}
	return gp
}
