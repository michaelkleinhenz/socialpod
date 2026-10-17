package handlers

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Duplicate detection and the agent draft queue: what an agent running on a
// schedule needs so it neither covers a story twice nor floods the review
// list.

// defaultMCPOrigin marks posts and drafts created over MCP when the client
// does not name itself.
const defaultMCPOrigin = "mcp"

var originRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// mcpOrigin is the origin label a creating tool call stores: the "origin"
// argument when it is a short slug, else "mcp".
func mcpOrigin(args map[string]any) string {
	o := strings.ToLower(strings.TrimSpace(strArg(args, "origin")))
	if originRe.MatchString(o) {
		return o
	}
	return defaultMCPOrigin
}

// trackingParams are query parameters that never change which page a URL
// points to.
var trackingParams = map[string]bool{
	"fbclid": true, "gclid": true, "mc_cid": true, "mc_eid": true, "ref": true,
	"ref_src": true, "igshid": true, "si": true, "_ga": true, "yclid": true,
}

// sourceKey normalises a URL so the same story found twice compares equal:
// BGG games become "bgg:<id>" whatever the slug; otherwise scheme, "www.",
// "m.", fragment, tracking parameters and a trailing slash are dropped, the
// host is lower-cased and the remaining parameters sorted. It returns "" for
// anything that is not an http(s) URL.
func sourceKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if id := bggGameIDFromURL(raw); id != "" {
		return "bgg:" + id
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	path := strings.TrimRight(u.EscapedPath(), "/")
	q := u.Query()
	var keys []string
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || trackingParams[lk] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	key := host + path
	if len(parts) > 0 {
		key += "?" + strings.Join(parts, "&")
	}
	return key
}

func agentQueueToolDefinitions() []mcpTool {
	return []mcpTool{
		{
			Name: "find_existing",
			Description: "Check whether a story was already covered before drafting it: finds posts (any status), news drafts (open and posted) and episode drafts " +
				"with the same source (URLs are compared normalised: BGG games by ID, other pages without tracking parameters, www. or trailing slash), " +
				"and, with query, posts and drafts whose text contains all its words. Looks back days days (default 180).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"urls":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Source URLs to check (article, BGG page)"},
					"query": map[string]any{"type": "string", "description": "Words to look for in the text, e.g. a game title"},
					"days":  map[string]any{"type": "number", "description": "Look-back window in days (default 180, max 730)"},
				},
			},
		},
		{
			Name: "get_draft_queue",
			Description: "The review queue of drafts created over MCP or by agents (anything with an origin): how many are open, by origin and kind, the team's limit (maxAgentDrafts, 0 = none) " +
				"and how many more may be created. Creating drafts fails once the limit is reached, so check this before a run.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}

type existingMatch struct {
	Kind        string     `json:"kind"` // post, news_draft, episode_draft
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Origin      string     `json:"origin,omitempty"`
	MatchedBy   string     `json:"matchedBy"` // source or text
	Source      string     `json:"source,omitempty"`
	Excerpt     string     `json:"excerpt"`
	ScheduledAt *time.Time `json:"scheduledAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

func textQueryFilter(fields []string, query string) []bson.M {
	var and []bson.M
	for _, w := range strings.Fields(query) {
		re := primitive.Regex{Pattern: regexp.QuoteMeta(strings.Trim(w, `"'„“”`)), Options: "i"}
		var or []bson.M
		for _, f := range fields {
			or = append(or, bson.M{f: re})
		}
		and = append(and, bson.M{"$or": or})
	}
	return and
}

func (h *MCPHandler) toolFindExisting(c *gin.Context, args map[string]any) (any, bool) {
	urls := strSliceArg(args, "urls")
	if u := strArg(args, "url"); u != "" {
		urls = append(urls, u)
	}
	query := strings.TrimSpace(strArg(args, "query"))
	if len(urls) == 0 && query == "" {
		return map[string]string{"error": "give urls or query"}, true
	}
	days := intArg(args, "days", 180)
	if days > 730 {
		days = 730
	}
	since := time.Now().AddDate(0, 0, -days)

	var keys, bggIDs []string
	for _, u := range urls {
		if k := sourceKey(u); k != "" {
			keys = append(keys, k)
			if strings.HasPrefix(k, "bgg:") {
				bggIDs = append(bggIDs, strings.TrimPrefix(k, "bgg:"))
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	matches := []existingMatch{}
	seen := map[string]bool{}
	add := func(m existingMatch) {
		if seen[m.Kind+m.ID] {
			return
		}
		seen[m.Kind+m.ID] = true
		matches = append(matches, m)
	}
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(25)

	findPosts := func(extra bson.M, by string) {
		filter := mcpScopeFilter(c)
		filter["createdAt"] = bson.M{"$gte": since}
		for k, v := range extra {
			filter[k] = v
		}
		cur, err := h.DB.Posts().Find(ctx, filter, opts)
		if err != nil {
			return
		}
		var posts []models.Post
		if cur.All(ctx, &posts) != nil {
			return
		}
		for _, p := range posts {
			at := p.ScheduledAt
			add(existingMatch{Kind: "post", ID: p.ID.Hex(), Status: string(p.Status), Origin: p.Origin, MatchedBy: by,
				Source: p.SourceURL, Excerpt: firstLine(p.Content, 140), ScheduledAt: &at, CreatedAt: p.CreatedAt})
		}
	}
	findNews := func(extra bson.M, by string) {
		filter := mcpScopeFilter(c)
		filter["createdAt"] = bson.M{"$gte": since}
		for k, v := range extra {
			filter[k] = v
		}
		cur, err := h.DB.NewsDrafts().Find(ctx, filter, opts)
		if err != nil {
			return
		}
		var drafts []models.NewsDraft
		if cur.All(ctx, &drafts) != nil {
			return
		}
		for _, d := range drafts {
			status := "open"
			if d.Posted {
				status = "posted"
			}
			text := d.NewsTagline
			if text == "" {
				text = d.Content
			}
			add(existingMatch{Kind: "news_draft", ID: d.ID.Hex(), Status: status, Origin: d.Origin, MatchedBy: by,
				Source: d.ArticleURL, Excerpt: firstLine(text, 140), CreatedAt: d.CreatedAt})
		}
	}
	findEpisodes := func(extra bson.M, by string) {
		filter := mcpScopeFilter(c)
		filter["createdAt"] = bson.M{"$gte": since}
		for k, v := range extra {
			filter[k] = v
		}
		cur, err := h.DB.EpisodeDrafts().Find(ctx, filter, opts)
		if err != nil {
			return
		}
		var drafts []models.EpisodeDraft
		if cur.All(ctx, &drafts) != nil {
			return
		}
		for _, d := range drafts {
			status := "open"
			if d.Posted {
				status = "posted"
			}
			add(existingMatch{Kind: "episode_draft", ID: d.ID.Hex(), Status: status, Origin: d.Origin, MatchedBy: by,
				Source: d.LinkBGG, Excerpt: firstLine(d.EpisodeTitle+" "+d.GameNamePublisher, 140), CreatedAt: d.CreatedAt})
		}
	}

	if len(keys) > 0 {
		findPosts(bson.M{"sourceKey": bson.M{"$in": keys}}, "source")
		findNews(bson.M{"sourceKey": bson.M{"$in": keys}}, "source")
		// Drafts and posts from before source keys existed: the raw URL in
		// the article field or the text.
		var or []bson.M
		for _, u := range urls {
			if k := sourceKey(u); k != "" && !strings.HasPrefix(k, "bgg:") {
				hostPath := strings.SplitN(k, "?", 2)[0]
				re := primitive.Regex{Pattern: regexp.QuoteMeta(hostPath), Options: "i"}
				or = append(or, bson.M{"articleUrl": re}, bson.M{"content": re})
			}
		}
		for _, id := range bggIDs {
			re := primitive.Regex{Pattern: `boardgamegeek\.com/boardgame[^/]*/` + id + `(\D|$)`, Options: "i"}
			or = append(or, bson.M{"articleUrl": re}, bson.M{"content": re}, bson.M{"linkBGG": re}, bson.M{"episodeNews.bggLink": re})
		}
		if len(or) > 0 {
			findPosts(bson.M{"$or": or}, "source")
			findNews(bson.M{"$or": or}, "source")
			findEpisodes(bson.M{"$or": or}, "source")
		}
	}
	if query != "" {
		if and := textQueryFilter([]string{"content", "contentOverrides.instagram", "firstComment"}, query); len(and) > 0 {
			findPosts(bson.M{"$and": and}, "text")
		}
		if and := textQueryFilter([]string{"newsTagline", "content", "shownotes"}, query); len(and) > 0 {
			findNews(bson.M{"$and": and}, "text")
		}
		if and := textQueryFilter([]string{"episodeTitle", "gameNamePublisher", "summary", "content"}, query); len(and) > 0 {
			findEpisodes(bson.M{"$and": and}, "text")
		}
	}
	return map[string]any{
		"found":   len(matches) > 0,
		"since":   since.Format(dateLayout),
		"matches": matches,
	}, false
}

// draftQueue counts the open drafts that carry an origin.
type draftQueue struct {
	Open           int            `json:"open"`
	ByOrigin       map[string]int `json:"byOrigin"`
	ByKind         map[string]int `json:"byKind"`
	MaxAgentDrafts int            `json:"maxAgentDrafts"`
	// Remaining is how many more drafts may be created; -1 means no limit.
	Remaining int `json:"remaining"`
}

func (h *MCPHandler) loadDraftQueue(ctx context.Context, c *gin.Context) (draftQueue, error) {
	q := draftQueue{ByOrigin: map[string]int{}, ByKind: map[string]int{}, Remaining: -1}
	// Quick Reel drafts carry an origin too, but a person made them.
	hasOrigin := bson.M{"$exists": true, "$nin": bson.A{"", quickReelOrigin}}
	proj := options.Find().SetProjection(bson.M{"origin": 1})

	postFilter := mcpScopeFilter(c)
	postFilter["status"] = models.PostStatusDraft
	postFilter["origin"] = hasOrigin
	newsFilter := mcpScopeFilter(c)
	applyPostedFilter(newsFilter, false)
	newsFilter["origin"] = hasOrigin
	epFilter := mcpScopeFilter(c)
	applyPostedFilter(epFilter, false)
	epFilter["origin"] = hasOrigin

	for _, src := range []struct {
		kind   string
		coll   *mongo.Collection
		filter bson.M
	}{
		{"post", h.DB.Posts(), postFilter},
		{"news_draft", h.DB.NewsDrafts(), newsFilter},
		{"episode_draft", h.DB.EpisodeDrafts(), epFilter},
	} {
		cur, err := src.coll.Find(ctx, src.filter, proj)
		if err != nil {
			return q, err
		}
		var docs []struct {
			Origin string `bson:"origin"`
		}
		if err := cur.All(ctx, &docs); err != nil {
			return q, err
		}
		for _, d := range docs {
			q.Open++
			q.ByOrigin[d.Origin]++
			q.ByKind[src.kind]++
		}
	}

	if teamID := teamIDFrom(c); teamID != nil {
		if settings, err := h.growth().loadGrowthSettings(ctx, *teamID); err == nil && settings.MaxAgentDrafts > 0 {
			q.MaxAgentDrafts = settings.MaxAgentDrafts
			q.Remaining = max(settings.MaxAgentDrafts-q.Open, 0)
		}
	}
	return q, nil
}

func (h *MCPHandler) toolGetDraftQueue(c *gin.Context, _ map[string]any) (any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q, err := h.loadDraftQueue(ctx, c)
	if err != nil {
		return map[string]string{"error": "Failed to count drafts"}, true
	}
	return q, false
}

// checkDraftCap refuses a new draft once the team's agent draft limit is
// reached. A count that fails lets the draft through: the cap is a brake on
// runaway agents, not a guarantee.
func (h *MCPHandler) checkDraftCap(c *gin.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q, err := h.loadDraftQueue(ctx, c)
	if err != nil || q.MaxAgentDrafts == 0 || q.Remaining > 0 {
		return nil
	}
	return fmt.Errorf("the draft queue is full (%d of %d open drafts from MCP and agents): wait until people have reviewed some, or ask a team admin to raise the limit", q.Open, q.MaxAgentDrafts)
}
