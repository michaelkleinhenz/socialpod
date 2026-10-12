package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Text tools for agents: the capture pipeline (URL to finished draft, in the
// team's house style) and the Instagram caption rewrite and checks, so text an
// agent writes follows the same rules as text written in the editor.

func textToolDefinitions() []mcpTool {
	return []mcpTool{
		{
			Name: "capture_url",
			Description: "Turn a web page into a finished draft with the same pipeline as the Chrome extension: the team's capture prompt, house style " +
				"(game titles in double quotes, no em dash), and for BoardGameGeek links the BGG data and the processed cover with the team overlay. " +
				"For other pages the article's main image is fetched and overlaid. entityType news creates a news draft, post a post draft, episode an episode draft. " +
				"Check find_existing first; the draft counts towards the agent draft limit.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":         map[string]any{"type": "string", "description": "Page to capture"},
					"entityType":  map[string]any{"type": "string", "enum": []string{"news", "post", "episode"}, "description": "What to create (default news)"},
					"description": map[string]any{"type": "string", "description": "Extra instructions for the text, e.g. the angle to take"},
					"origin":      map[string]any{"type": "string", "description": "Origin label of the created draft (default mcp)"},
				},
				"required": []string{"url"},
			},
		},
		{
			Name: "rewrite_instagram",
			Description: "Rewrite a text as an Instagram caption with the team's rewrite prompt (hook, closing question, share prompt, hashtags), " +
				"like the editor's \"Rewrite for Instagram\" button. Returns text.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string", "description": "Text to rewrite, e.g. the Bluesky text"}},
				"required":   []string{"text"},
			},
		},
		{
			Name: "check_instagram_caption",
			Description: "Check an Instagram caption against the growth plan's rules, like the editor's caption checker: hook in the first line (10-90 characters), " +
				"ends with a question, share prompt, 3-5 hashtags, a tagged account, not identical to the Bluesky text. Each check is ok, warn or missing, with a hint.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text":          map[string]any{"type": "string", "description": "The Instagram caption"},
					"blueskyText":   map[string]any{"type": "string", "description": "The post's Bluesky text, for the copy check"},
					"collaborators": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Instagram collaborators/image tags of the post (they count as tagged accounts)"},
				},
				"required": []string{"text"},
			},
		},
	}
}

func (h *MCPHandler) toolCaptureURL(c *gin.Context, args map[string]any) (any, bool) {
	src := strings.TrimSpace(strArg(args, "url"))
	if src == "" {
		return map[string]string{"error": "url is required"}, true
	}
	entity := strArg(args, "entityType")
	if entity == "" {
		entity = "news"
	}
	if err := h.checkDraftCap(c); err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	capture := &CaptureHandler{DB: h.DB, UploadDir: h.UploadDir, BGG: h.BGG}
	status, out, err := callREST(c, capture.Capture, http.MethodPost, "/api/capture", CaptureInput{
		URL:         src,
		EntityType:  entity,
		Description: strArg(args, "description"),
	})
	if err != nil || status != http.StatusOK {
		return restResult(status, out, err)
	}
	draft, _ := out["draft"].(map[string]any)
	idHex, _ := draft["id"].(string)
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return out, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	set := bson.M{"origin": mcpOrigin(args)}
	var coll *mongo.Collection
	switch entity {
	case "post":
		coll = h.DB.Posts()
		set["sourceUrl"] = src
		set["sourceKey"] = sourceKey(src)
	case "news":
		coll = h.DB.NewsDrafts()
		set["sourceKey"] = sourceKey(src)
	default:
		coll = h.DB.EpisodeDrafts()
	}
	// The extension sends the page's images along; over MCP there are none,
	// so a non-BGG news or post capture fetches the article image itself.
	if entity != "episode" && len(anyStrings(draft["imageUrls"])) == 0 && bggGameIDFromURL(src) == "" {
		if urls := h.fetchArticleImage(c, src); len(urls) > 0 {
			set["imageUrls"] = urls
			draft["imageUrls"] = urls
		}
	}
	if _, err := coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err == nil {
		for k, v := range set {
			if k != "imageUrls" {
				draft[k] = v
			}
		}
	}
	return out, false
}

func (h *MCPHandler) toolRewriteInstagram(c *gin.Context, args map[string]any) (any, bool) {
	status, out, err := callREST(c, h.growth().RewriteInstagram, http.MethodPost, "/api/growth/rewrite-instagram", map[string]string{"text": strArg(args, "text")})
	return restResult(status, out, err)
}

func (h *MCPHandler) toolCheckInstagramCaption(_ *gin.Context, args map[string]any) (any, bool) {
	text := strArg(args, "text")
	if strings.TrimSpace(text) == "" {
		return map[string]string{"error": "text is required"}, true
	}
	var bluesky *string
	if v, ok := args["blueskyText"].(string); ok {
		bluesky = &v
	}
	tagged := len(extractIGHandles(text)) + len(cleanUsernames(strSliceArg(args, "collaborators"), 20))
	checks := checkCaption(text, bluesky, tagged)
	ok := true
	for _, ch := range checks {
		if ch.Level != "ok" {
			ok = false
		}
	}
	return map[string]any{"allOk": ok, "checks": checks, "length": len([]rune(text)), "limit": instagramCharLimit}, false
}
