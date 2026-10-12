package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// Tools for a content agent that fills the draft queue on its own: BGG
// imports, imports from a URL, and the team's weekly plan, so the agent knows
// what to make and when to schedule it.

func agentToolDefinitions() []mcpTool {
	return []mcpTool{
		{
			Name: "import_bgg",
			Description: "Import a board game from BoardGameGeek, like the editor's \"import from BGG\" button: title, year, players, playtime, designers, artists, publishers, categories, mechanics, rating, weight, description, " +
				"an AI summary and abstract, suggested post text (per platform with resolved @handles when the team has handle lookup on), suggested hashtags and the publishers' Instagram handles (use them as collaborators). " +
				"The cover is letterboxed to 1080x1080 with the team's overlay and stored as an upload; its URL is returned as imageUrl. " +
				"With createDraft=true a post draft is created from it right away and returned as post.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":         map[string]any{"type": "string", "description": "BoardGameGeek game URL, e.g. https://boardgamegeek.com/boardgame/266192/wingspan"},
					"overlay":     map[string]any{"type": "string", "enum": []string{"bgg", "news_entry", "news", "review", "special", "tatort"}, "description": "Which team overlay goes on the cover: bgg (default, the team's BGG watermark), news_entry (the news creator watermark), or an episode type's overlay"},
					"createDraft": map[string]any{"type": "boolean", "description": "Also create a post draft with the suggested text, the cover, the hashtags and the publishers as Instagram collaborators"},
					"platforms":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Platforms of the created draft (default: instagram, bluesky)"},
					"scheduledAt": map[string]any{"type": "string", "description": "Planned time of the created draft (RFC3339); defaults to tomorrow at the same time"},
					"sourceUrl":   map[string]any{"type": "string", "description": "Source the draft is about (defaults to the BGG URL); used to detect duplicates"},
					"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags of the created draft"},
					"origin":      map[string]any{"type": "string", "description": "Origin label of the created draft (default mcp)"},
				},
				"required": []string{"url"},
			},
		},
		{
			Name: "upload_from_url",
			Description: "Download an image or video from a public URL and store it as an upload, without passing the data through the conversation. " +
				"Returns url (relative, for imageUrls) and absoluteUrl. Use it for images found on the web before composing or attaching them.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "description": "Public http(s) URL of the file"},
				},
				"required": []string{"url"},
			},
		},
		{
			Name: "get_growth_settings",
			Description: "The team's growth settings, read-only: the Instagram follower goal, the weekly content plan (recurring slots: weekday 0 = Monday, optional HH:MM time, format reel/carousel/image/story/any, label), " +
				"the single-image limit per week and the Instagram rewrite prompt. Use it to decide which formats to make.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name: "get_open_slots",
			Description: "The weekly-plan slots of the coming days that no Instagram post (draft or scheduled, not failed) fills yet, each with its date, time, format and label. " +
				"Schedule new drafts into these. A post fills at most one slot on its day; exact formats are matched before \"any\", the post closest in time wins.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"days":     map[string]any{"type": "number", "description": "How many days ahead, starting today (default 7, max 28)"},
					"timezone": map[string]any{"type": "string", "description": "IANA time zone the plan's times are in (default Europe/Berlin)"},
				},
			},
		},
	}
}

// absoluteURL turns an upload path into a full URL using APP_URL, or the
// request's host when APP_URL is not set.
func absoluteURL(c *gin.Context, path string) string {
	if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	base := strings.TrimRight(appBaseURL, "/")
	if base == "" && c != nil && c.Request != nil {
		scheme := "https"
		if c.Request.TLS == nil && c.GetHeader("X-Forwarded-Proto") == "" {
			scheme = "http"
		} else if p := c.GetHeader("X-Forwarded-Proto"); p != "" {
			scheme = p
		}
		base = scheme + "://" + c.Request.Host
	}
	return base + path
}

// appBaseURL is APP_URL; main sets it at startup.
var appBaseURL string

// SetAppBaseURL records the public URL tools use for absolute links.
func SetAppBaseURL(u string) { appBaseURL = u }

func (h *MCPHandler) toolImportBGG(c *gin.Context, args map[string]any) (any, bool) {
	rawURL := strings.TrimSpace(strArg(args, "url"))
	if rawURL == "" {
		return map[string]string{"error": "url is required"}, true
	}
	if h.BGG == nil {
		return map[string]string{"error": "BGG import is not available"}, true
	}
	// Refuse before the import stores a cover nothing would use.
	if boolArg(args, "createDraft") {
		if err := h.checkDraftCap(c); err != nil {
			return map[string]string{"error": err.Error()}, true
		}
	}
	overlay := strArg(args, "overlay")
	if overlay == "bgg" {
		overlay = ""
	}
	q := url.Values{"url": {rawURL}}
	if overlay != "" {
		q.Set("episodeType", overlay)
	}
	status, out, err := callREST(c, h.BGG.FetchGame, http.MethodGet, "/api/bgg/fetch?"+q.Encode(), nil)
	if err != nil || status != http.StatusOK {
		return restResult(status, out, err)
	}

	// The cover comes back as base64; store it instead of sending it on.
	imageURL := ""
	if b64, _ := out["imageBase64"].(string); b64 != "" {
		if data, err := base64.StdEncoding.DecodeString(b64); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			name := fmt.Sprintf("%d.jpg", time.Now().UnixNano())
			if u, err := storeUploadBytes(ctx, h.DB, h.UploadDir, name, "image/jpeg", data); err == nil {
				imageURL = u
			}
		}
	}
	delete(out, "imageBase64")
	delete(out, "imageFilename")
	out["imageUrl"] = imageURL
	out["absoluteImageUrl"] = absoluteURL(c, imageURL)

	if !boolArg(args, "createDraft") {
		return out, false
	}

	content, _ := out["suggestedContent"].(string)
	platforms := strSliceArg(args, "platforms")
	if len(platforms) == 0 {
		platforms = []string{"instagram", "bluesky"}
	}
	scheduledAt := strArg(args, "scheduledAt")
	if scheduledAt == "" {
		scheduledAt = time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	}
	sourceURL := strArg(args, "sourceUrl")
	if sourceURL == "" {
		sourceURL = rawURL
	}
	postArgs := map[string]any{
		"content":     content,
		"platforms":   toAnySlice(platforms),
		"scheduledAt": scheduledAt,
		"status":      "draft",
		"sourceUrl":   sourceURL,
	}
	if tags, ok := args["tags"]; ok {
		postArgs["tags"] = tags
	}
	if origin, ok := args["origin"]; ok {
		postArgs["origin"] = origin
	}
	if imageURL != "" {
		postArgs["imageUrls"] = []any{imageURL}
	}
	// Like the editor's BGG import: the handle-resolved text per platform
	// where there is one, and the hashtags on Instagram and X only.
	byPlatform, _ := out["suggestedContentByPlatform"].(map[string]any)
	tags := hashtagLine(anyStrings(out["suggestedHashtags"]))
	overrides := map[string]any{}
	for _, p := range platforms {
		text := content
		if s, ok := byPlatform[p].(string); ok && s != "" {
			text = s
		}
		if tags != "" && (p == "instagram" || p == "twitter") {
			text = strings.TrimRight(text, "\n") + "\n\n" + tags
		}
		if text != content {
			overrides[p] = text
		}
	}
	if len(overrides) > 0 {
		postArgs["contentOverrides"] = overrides
	}
	if handles := anyStrings(out["publisherInstagramHandles"]); len(handles) > 0 {
		postArgs["collaborators"] = toAnySlice(handles)
	}
	post, isErr := h.toolCreatePost(c, postArgs)
	if isErr {
		out["draftError"] = post
		return out, true
	}
	out["post"] = post
	return out, false
}

func hashtagLine(tags []string) string {
	parts := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "#") {
			t = "#" + t
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, " ")
}

func anyStrings(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func (h *MCPHandler) toolUploadFromURL(c *gin.Context, args map[string]any) (any, bool) {
	src := strings.TrimSpace(strArg(args, "url"))
	if src == "" {
		return map[string]string{"error": "url is required"}, true
	}
	posts := &PostHandler{DB: h.DB, UploadDir: h.UploadDir}
	status, out, err := callREST(c, posts.UploadFromURL, http.MethodPost, "/api/upload-from-url", map[string]string{"url": src})
	if err == nil && status == http.StatusOK {
		if u, _ := out["url"].(string); u != "" {
			out["absoluteUrl"] = absoluteURL(c, u)
		}
	}
	return restResult(status, out, err)
}

func (h *MCPHandler) toolGetGrowthSettings(c *gin.Context, _ map[string]any) (any, bool) {
	status, out, err := callREST(c, h.growth().GetSettings, http.MethodGet, "/api/growth/settings", nil)
	return restResult(status, out, err)
}

// OpenSlot is a weekly-plan slot on a concrete day that no post fills yet.
type OpenSlot struct {
	Date    string `json:"date"`
	Weekday int    `json:"weekday"`
	Time    string `json:"time,omitempty"`
	// At is the slot's moment in RFC3339 (noon when the slot has no time),
	// ready to pass as scheduledAt.
	At     string `json:"at"`
	Format string `json:"format"`
	Label  string `json:"label,omitempty"`
	SlotID string `json:"slotId"`
}

func (h *MCPHandler) toolGetOpenSlots(c *gin.Context, args map[string]any) (any, bool) {
	days := intArg(args, "days", 7)
	if days > 28 {
		days = 28
	}
	tzName := strArg(args, "timezone")
	if tzName == "" {
		tzName = "Europe/Berlin"
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return map[string]string{"error": "Unknown time zone " + tzName}, true
	}
	teamID := teamIDFrom(c)
	if teamID == nil {
		return map[string]any{"slots": []OpenSlot{}, "note": "The weekly plan needs a team"}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	settings, err := h.growth().loadGrowthSettings(ctx, *teamID)
	if err != nil {
		return map[string]string{"error": "Team not found"}, true
	}

	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, days)
	filter := mcpScopeFilter(c)
	filter["platforms"] = models.PlatformInstagram
	filter["status"] = bson.M{"$ne": models.PostStatusFailed}
	filter["scheduledAt"] = bson.M{"$gte": start, "$lt": end}
	cursor, err := h.DB.Posts().Find(ctx, filter)
	if err != nil {
		return map[string]string{"error": "Failed to load posts"}, true
	}
	var posts []models.Post
	if err := cursor.All(ctx, &posts); err != nil {
		return map[string]string{"error": "Failed to load posts"}, true
	}
	return map[string]any{
		"timezone": tzName,
		"slots":    openSlots(settings.WeeklyPlan, posts, start, days, now, loc),
	}, false
}

// openSlots lists the plan slots of the days from start on that the posts
// leave open, skipping slots already past. It mirrors openSlotsForDay in the
// frontend's instagramGrowth.ts.
func openSlots(plan []models.WeeklyPlanSlot, posts []models.Post, start time.Time, days int, now time.Time, loc *time.Location) []OpenSlot {
	out := []OpenSlot{}
	for d := 0; d < days; d++ {
		day := start.AddDate(0, 0, d)
		weekday := (int(day.Weekday()) + 6) % 7
		var daySlots []models.WeeklyPlanSlot
		for _, s := range plan {
			if s.Weekday == weekday {
				daySlots = append(daySlots, s)
			}
		}
		sort.SliceStable(daySlots, func(i, j int) bool {
			return daySlots[i].Format != models.SlotFormatAny && daySlots[j].Format == models.SlotFormatAny
		})
		var remaining []models.Post
		for _, p := range posts {
			t := p.ScheduledAt.In(loc)
			if t.Year() == day.Year() && t.YearDay() == day.YearDay() {
				remaining = append(remaining, p)
			}
		}
		for _, slot := range daySlots {
			target := slotMinutes(slot.Time)
			best, bestDist := -1, 1<<30
			for i, p := range remaining {
				if !slotAccepts(slot.Format, instagramFormat(p)) {
					continue
				}
				dist := 0
				if target >= 0 {
					t := p.ScheduledAt.In(loc)
					dist = absInt(t.Hour()*60 + t.Minute() - target)
				}
				if dist < bestDist {
					best, bestDist = i, dist
				}
			}
			if best >= 0 {
				remaining = append(remaining[:best], remaining[best+1:]...)
				continue
			}
			mins := target
			if mins < 0 {
				mins = 12 * 60
			}
			at := day.Add(time.Duration(mins) * time.Minute)
			if at.Before(now) {
				continue
			}
			out = append(out, OpenSlot{
				Date:    day.Format(dateLayout),
				Weekday: weekday,
				Time:    slot.Time,
				At:      at.Format(time.RFC3339),
				Format:  slot.Format,
				Label:   slot.Label,
				SlotID:  slot.ID,
			})
		}
	}
	return out
}

func slotAccepts(slotFormat, postFormat string) bool {
	if slotFormat == models.SlotFormatAny {
		return postFormat != formatStory
	}
	return slotFormat == postFormat
}

// slotMinutes reads "HH:MM" as minutes after midnight, -1 when unset.
func slotMinutes(s string) int {
	var hh, mm int
	if _, err := fmt.Sscanf(s, "%d:%d", &hh, &mm); err != nil {
		return -1
	}
	return hh*60 + mm
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
