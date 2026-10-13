package handlers

import (
	"os"
	"strings"
	"testing"
	"time"

	"socialmedia/internal/models"
	"socialmedia/internal/services"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/image/font/gofont/goregular"
)

func snap(date string, n int) models.FollowerSnapshot {
	return models.FollowerSnapshot{Date: date, FollowersCount: n}
}

func igPost(postType models.PostType, images int, postedAt time.Time, ins *models.PostInsights) models.Post {
	urls := make([]string, images)
	for i := range urls {
		urls[i] = "/api/uploads/x.jpg"
	}
	return models.Post{
		ID:        primitive.NewObjectID(),
		PostType:  postType,
		ImageURLs: urls,
		Content:   "Hook line\nmore",
		Results: []models.PostResult{{
			Platform: models.PlatformInstagram, Success: true, PostID: "1", PostedAt: postedAt, Insights: ins,
		}},
	}
}

func insights(reach, shares, saved int) *models.PostInsights {
	return &models.PostInsights{Reach: reach, Shares: shares, Saved: saved, FetchedAt: time.Now()}
}

func TestComputeGoalProgress(t *testing.T) {
	goal := models.InstagramGoal{Target: 1000, StartDate: "2026-10-12", EndDate: "2027-01-04"} // 12 weeks
	snaps := []models.FollowerSnapshot{
		snap("2026-10-11", 496), // the day before the start is the start count
		snap("2026-10-12", 500),
		snap("2026-10-18", 540), // end of week 1: +44
		snap("2026-10-25", 640), // end of week 2: +100
		snap("2026-11-22", 700),
	}

	gp := computeGoalProgress(goal, snaps, "2026-11-23")
	if gp.StartFollowers == nil || *gp.StartFollowers != 496 {
		t.Fatalf("start followers = %v, want 496", gp.StartFollowers)
	}
	if gp.RequiredPerWeek != 42 {
		t.Errorf("required per week = %v, want 42 ((1000-496)/12)", gp.RequiredPerWeek)
	}
	if len(gp.Weeks) != 12 {
		t.Fatalf("got %d weeks, want 12", len(gp.Weeks))
	}
	if gp.Weeks[0].Net == nil || *gp.Weeks[0].Net != 44 {
		t.Errorf("week 1 net = %v, want 44", gp.Weeks[0].Net)
	}
	if gp.Weeks[1].Net == nil || *gp.Weeks[1].Net != 100 {
		t.Errorf("week 2 net = %v, want 100", gp.Weeks[1].Net)
	}
	if gp.Weeks[2].Net != nil {
		t.Errorf("week 3 has no snapshot, net = %v, want nil", *gp.Weeks[2].Net)
	}
	// Halfway defaults to the midpoint (2026-11-23) and 40%: by then the
	// account had 700-496 = 204 of 504, about 40.5%.
	if gp.HalfwayDate != "2026-11-23" {
		t.Errorf("halfway date = %s, want 2026-11-23", gp.HalfwayDate)
	}
	if gp.Halfway != "ok" {
		t.Errorf("halfway = %s, want ok", gp.Halfway)
	}

	lower := append([]models.FollowerSnapshot{}, snaps[:4]...)
	lower = append(lower, snap("2026-11-22", 650))
	if gp := computeGoalProgress(goal, lower, "2026-11-24"); gp.Halfway != "below" {
		t.Errorf("halfway = %s with 154/504, want below", gp.Halfway)
	}
	if gp := computeGoalProgress(goal, snaps, "2026-11-01"); gp.Halfway != "pending" {
		t.Errorf("halfway = %s before the halfway date, want pending", gp.Halfway)
	}

	override := 300
	goal.StartFollowers = &override
	if gp := computeGoalProgress(goal, snaps, "2026-11-23"); *gp.StartFollowers != 300 || gp.RequiredPerWeek != 58.3 {
		t.Errorf("override: start %d, per week %v; want 300 and 58.3", *gp.StartFollowers, gp.RequiredPerWeek)
	}
}

func TestComputeGoalProgressWithoutSnapshots(t *testing.T) {
	goal := models.InstagramGoal{Target: 1000, StartDate: "2026-10-12", EndDate: "2027-01-04"}
	gp := computeGoalProgress(goal, nil, "2026-10-12")
	if gp.StartFollowers != nil || gp.CurrentFollowers != nil || len(gp.Weeks) != 0 {
		t.Fatalf("expected an empty tracker without snapshots, got %+v", gp)
	}
}

func TestInstagramFormat(t *testing.T) {
	now := time.Now()
	cases := []struct {
		post models.Post
		want string
	}{
		{igPost(models.PostTypeReel, 1, now, nil), formatReel},
		{igPost(models.PostTypeStory, 1, now, nil), formatStory},
		{igPost(models.PostTypePost, 5, now, nil), formatCarousel},
		{igPost(models.PostTypePost, 1, now, nil), formatImage},
	}
	for _, tc := range cases {
		if got := instagramFormat(tc.post); got != tc.want {
			t.Errorf("format = %s, want %s", got, tc.want)
		}
	}
}

func TestBuildWeeklyReport(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	day := func(d int) time.Time { return start.AddDate(0, 0, d).Add(9 * time.Hour) }

	posts := []models.Post{
		igPost(models.PostTypeReel, 1, day(1), insights(2000, 40, 20)),    // 60
		igPost(models.PostTypePost, 6, day(0), insights(800, 10, 50)),     // 60, less reach
		igPost(models.PostTypePost, 1, day(3), insights(500, 1, 2)),       // 3
		igPost(models.PostTypePost, 1, day(4), insights(400, 0, 1)),       // 1
		igPost(models.PostTypeReel, 1, day(5), nil),                       // no insights yet
		igPost(models.PostTypeStory, 1, day(2), insights(100, 0, 0)),      // reported apart
		igPost(models.PostTypeReel, 1, day(-1), insights(9000, 500, 500)), // previous week
	}
	snaps := []models.FollowerSnapshot{snap("2026-10-04", 480), snap("2026-10-11", 530)}

	rep := buildWeeklyReport(posts, start, end, snaps, map[string]bool{posts[0].ID.Hex(): true})
	if len(rep.Posts) != 5 {
		t.Fatalf("report holds %d posts, want 5", len(rep.Posts))
	}
	if rep.Stories != 1 || rep.MissingInsights != 1 {
		t.Errorf("stories %d, missing %d; want 1 and 1", rep.Stories, rep.MissingInsights)
	}
	if len(rep.TopPosts) != 3 || rep.TopPosts[0].Format != formatReel || rep.TopPosts[1].Format != formatCarousel {
		t.Errorf("top posts out of order: %+v", rep.TopPosts)
	}
	if !rep.TopPosts[0].AdCandidate {
		t.Errorf("the flagged post lost its ad-candidate flag")
	}
	if rep.WeakestFormat != formatImage {
		t.Errorf("weakest format = %s, want image", rep.WeakestFormat)
	}
	for _, f := range rep.Formats {
		if f.Format == formatImage && (f.Posts != 2 || f.Reach != 900 || f.SendsPerReach != 0.0011) {
			t.Errorf("image stats = %+v", f)
		}
	}
	if rep.NetFollowers == nil || *rep.NetFollowers != 50 {
		t.Errorf("net followers = %v, want 50", rep.NetFollowers)
	}
}

func TestMarkAdCandidates(t *testing.T) {
	perfs := []PostPerformance{
		{PostID: "a", Format: formatReel, HasInsights: true, Score: 100},
		{PostID: "b", Format: formatCarousel, HasInsights: true, Score: 80},
		{PostID: "c", Format: formatReel, HasInsights: true, Score: 10},
		{PostID: "d", Format: formatReel, HasInsights: true, Score: 5},
		{PostID: "e", Format: formatCarousel, HasInsights: true, Score: 1},
		{PostID: "f", Format: formatCarousel, HasInsights: true, Score: 0},
		{PostID: "g", Format: formatImage, HasInsights: true, Score: 500}, // images are never advertised
		{PostID: "h", Format: formatReel, HasInsights: false, Score: 0},
	}
	markAdCandidates(perfs)
	var got []string
	for _, p := range perfs {
		if p.AdCandidate {
			got = append(got, p.PostID)
		}
	}
	// Five eligible posts with a score: the top 20% is one post.
	if strings.Join(got, ",") != "a" {
		t.Errorf("ad candidates = %v, want [a]", got)
	}
}

func TestExtractIGHandles(t *testing.T) {
	text := "Neu von @feuerland.spiele und @Frosted_Games. Mail: info@example.com, nochmal @feuerland.spiele"
	got := extractIGHandles(text)
	if strings.Join(got, ",") != "feuerland.spiele,Frosted_Games" {
		t.Errorf("handles = %v", got)
	}
}

func TestRadarCaption(t *testing.T) {
	drafts := []models.NewsDraft{
		{NewsTagline: "Arche Nova Erweiterung", GermanRelease: "ja", Content: "Neu bei @feuerlandspiele"},
		{Content: "Großes Spiel von @ludonova\nzweite Zeile", GermanRelease: "offen"},
	}
	radar := defaultGrowthSettings(nil).Radar
	caption, tags := radarCaption(drafts, *radar)
	for _, want := range []string{
		"1. Arche Nova Erweiterung · Kommt's auf Deutsch? Ja (@feuerlandspiele)",
		"2. Großes Spiel von · Kommt's auf Deutsch? Offen (@ludonova)",
		defaultRadarCaptionOutro,
	} {
		if !strings.Contains(caption, want) {
			t.Errorf("caption misses %q:\n%s", want, caption)
		}
	}
	if strings.Join(tags, ",") != "feuerlandspiele,ludonova" {
		t.Errorf("tags = %v", tags)
	}
}

func TestValidateGrowthSettings(t *testing.T) {
	ok := models.TeamGrowthSettings{
		InstagramGoal: &models.InstagramGoal{Target: 1000, StartDate: "2026-10-12", EndDate: "2027-01-04"},
		WeeklyPlan:    []models.WeeklyPlanSlot{{Weekday: 2, Time: "06:00", Format: "reel"}},
	}
	if err := validateGrowthSettings(&ok); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	stale := models.TeamGrowthSettings{Radar: &models.RadarSettings{FontName: "Old.ttf"}}
	if err := validateGrowthSettings(&stale); err != nil || stale.Radar.FontName != "" {
		t.Errorf("a font name without a font should be cleared, got %q (%v)", stale.Radar.FontName, err)
	}
	if ok.WeeklyPlan[0].ID == "" {
		t.Errorf("a new slot should get an ID")
	}
	bad := []models.TeamGrowthSettings{
		{InstagramGoal: &models.InstagramGoal{Target: 1000, StartDate: "2027-01-04", EndDate: "2026-10-12"}},
		{WeeklyPlan: []models.WeeklyPlanSlot{{Weekday: 7, Format: "reel"}}},
		{WeeklyPlan: []models.WeeklyPlanSlot{{Weekday: 1, Time: "6 Uhr", Format: "reel"}}},
		{WeeklyPlan: []models.WeeklyPlanSlot{{Weekday: 1, Format: "podcast"}}},
		{Radar: &models.RadarSettings{CoverImageURL: "https://elsewhere.example/x.jpg"}},
		{Radar: &models.RadarSettings{FontURL: "https://elsewhere.example/font.ttf"}},
	}
	for i, s := range bad {
		if err := validateGrowthSettings(&s); err == nil {
			t.Errorf("case %d: invalid settings accepted", i)
		}
	}
}

func TestCheckRadarFont(t *testing.T) {
	// A CFF-flavoured OpenType font: a subset of Inter Bold (testdata/README).
	otf, err := os.ReadFile("testdata/radar-font.otf")
	if err != nil {
		t.Fatal(err)
	}
	accepted := []struct {
		name     string
		data     []byte
		ext, typ string
	}{
		{"Inter-Bold.otf", otf, ".otf", "font/otf"},
		{"INTER-BOLD.OTF", otf, ".otf", "font/otf"},
		{"GoRegular.ttf", goregular.TTF, ".ttf", "font/ttf"},
		{"GoRegular.TTF", goregular.TTF, ".ttf", "font/ttf"},
		// The stored type follows the contents, not the name.
		{"TrueTypeOutlines.otf", goregular.TTF, ".ttf", "font/ttf"},
		{"CFFOutlines.ttf", otf, ".otf", "font/otf"},
	}
	for _, tc := range accepted {
		ext, typ, err := checkRadarFont(tc.name, tc.data)
		if err != nil {
			t.Errorf("%s rejected: %v", tc.name, err)
			continue
		}
		if ext != tc.ext || typ != tc.typ {
			t.Errorf("%s stored as %s %s, want %s %s", tc.name, ext, typ, tc.ext, tc.typ)
		}
	}

	// CFF outlines take a different path from TrueType ones; draw every text
	// a Radar has with them.
	fonts, err := services.ParseRadarFont(otf)
	if err != nil {
		t.Fatal(err)
	}
	colors := services.DefaultRadarColors()
	if _, err := services.RenderRadarCover(nil, "Radar der Woche", "KW 42 · 5 Ankündigungen", colors, fonts); err != nil {
		t.Errorf("cover with an OpenType font: %v", err)
	}
	if _, err := services.RenderRadarItem(services.RadarItemSlide{Index: 1, Total: 5, Title: "Größenwahn", GermanRelease: "offen"},
		"Radar der Woche", "Kommt's auf Deutsch?", colors, fonts); err != nil {
		t.Errorf("item slide with an OpenType font: %v", err)
	}

	rejected := []struct {
		name string
		data []byte
	}{
		{"Inter-Bold.woff2", otf},
		{"font", goregular.TTF},
		{"broken.ttf", []byte("not a font at all")},
		{"collection.ttf", append([]byte("ttcf"), make([]byte, 64)...)},
		{"huge.otf", make([]byte, maxRadarFontSize+1)},
	}
	for _, tc := range rejected {
		if _, _, err := checkRadarFont(tc.name, tc.data); err == nil {
			t.Errorf("%s accepted", tc.name)
		}
	}
}
