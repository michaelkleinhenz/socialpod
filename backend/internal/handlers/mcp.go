package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"
	"socialmedia/internal/services"
	"socialmedia/internal/uploadstore"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MCPHandler struct {
	DB        *database.MongoDB
	UploadDir string
	BGG       *BGGHandler
	// Scheduler refreshes Instagram Insights on request; nil disables that.
	Scheduler *services.Scheduler
}

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string   `json:"jsonrpc"`
	ID      any      `json:"id,omitempty"`
	Result  any      `json:"result,omitempty"`
	Error   *rpcErr  `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolResultContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

func mcpScopeFilter(c *gin.Context) bson.M {
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		return bson.M{"teamId": tid}
	}
	userID, _ := c.Get("userId")
	uid, _ := primitive.ObjectIDFromHex(userID.(string))
	return bson.M{"userId": uid}
}

// HandleGet responds to GET /api/mcp. The Streamable HTTP transport uses GET
// to open an SSE stream for server-initiated messages. This implementation is
// stateless (no server-to-client notifications), so we accept the connection
// and close it immediately — enough to satisfy MCP clients that probe the
// endpoint before sending POST requests.
func (h *MCPHandler) HandleGet(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	// Send an empty SSE comment as a keepalive, then close.
	c.Writer.WriteString(": ok\n\n")
	c.Writer.Flush()
}

// HandleDelete responds to DELETE /api/mcp. Clients call this to close an MCP
// session. Since the server is stateless, we simply acknowledge.
func (h *MCPHandler) HandleDelete(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (h *MCPHandler) Handle(c *gin.Context) {
	var req jsonrpcRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, jsonrpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcErr{Code: -32700, Message: "Parse error"},
		})
		return
	}

	if req.JSONRPC != "2.0" {
		c.JSON(http.StatusBadRequest, jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcErr{Code: -32600, Message: "Invalid request: jsonrpc must be \"2.0\""},
		})
		return
	}

	if req.ID == nil {
		c.Status(http.StatusAccepted)
		return
	}

	var resp jsonrpcResponse
	switch req.Method {
	case "initialize":
		resp = h.handleInitialize(req.ID, req.Params)
	case "ping":
		resp = jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		resp = h.handleToolsList(req.ID)
	case "tools/call":
		resp = h.handleToolsCall(c, req.ID, req.Params)
	default:
		resp = jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcErr{Code: -32601, Message: "Method not found: " + req.Method},
		}
	}

	c.JSON(http.StatusOK, resp)
}

// mcpProtocolVersions are the MCP revisions this stateless JSON-only server
// speaks, newest first. All of them work the same way here: no sessions, no
// server-initiated messages, one JSON response per request.
var mcpProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// negotiateProtocolVersion answers with the client's requested revision when
// supported, else the newest one; the client then decides whether to proceed.
func negotiateProtocolVersion(params json.RawMessage) string {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	for _, v := range mcpProtocolVersions {
		if v == p.ProtocolVersion {
			return v
		}
	}
	return mcpProtocolVersions[0]
}

func (h *MCPHandler) handleInitialize(id any, params json.RawMessage) jsonrpcResponse {
	return jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"protocolVersion": negotiateProtocolVersion(params),
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "socialpod",
				"version": "1.0.0",
			},
		},
	}
}

func (h *MCPHandler) handleToolsList(id any) jsonrpcResponse {
	return jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  map[string]any{"tools": h.toolDefinitions()},
	}
}

func (h *MCPHandler) handleToolsCall(c *gin.Context, id any, params json.RawMessage) jsonrpcResponse {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &rpcErr{Code: -32602, Message: "Invalid params"},
		}
	}

	// Log tool call arguments for debugging image issues
	if call.Name == "create_news_draft" || call.Name == "update_news_draft" ||
		call.Name == "create_episode_draft" || call.Name == "update_episode_draft" ||
		call.Name == "create_post" || call.Name == "update_post" || call.Name == "upload_image" {
		argKeys := make([]string, 0, len(call.Arguments))
		for k, v := range call.Arguments {
			switch k {
			case "images":
				if arr, ok := v.([]any); ok {
					log.Printf("[MCP] tool=%s arg=%s count=%d", call.Name, k, len(arr))
					for i, item := range arr {
						if obj, ok := item.(map[string]any); ok {
							fn, _ := obj["filename"].(string)
							d, _ := obj["data"].(string)
							log.Printf("[MCP]   images[%d]: filename=%q dataLen=%d dataPrefix=%.40s", i, fn, len(d), d)
						} else {
							log.Printf("[MCP]   images[%d]: unexpected type %T", i, item)
						}
					}
				} else {
					log.Printf("[MCP] tool=%s arg=%s unexpected type %T", call.Name, k, v)
				}
			case "imageUrls":
				log.Printf("[MCP] tool=%s arg=%s value=%v", call.Name, k, v)
			default:
				argKeys = append(argKeys, k)
			}
		}
		log.Printf("[MCP] tool=%s otherArgs=%v", call.Name, argKeys)
	}

	result, isErr := h.callTool(c, call.Name, call.Arguments)

	// A rich result carries images: they follow the JSON as image blocks.
	var images []mcpImage
	if rich, ok := result.(*mcpRichResult); ok {
		result, images = rich.Result, rich.Images
	}
	text, _ := json.Marshal(result)
	content := []toolResultContent{{Type: "text", Text: string(text)}}
	for _, img := range images {
		content = append(content, toolResultContent{
			Type:     "image",
			Data:     base64.StdEncoding.EncodeToString(img.Data),
			MimeType: img.MimeType,
		})
	}
	return jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": content,
			"isError": isErr,
		},
	}
}

func (h *MCPHandler) callTool(c *gin.Context, name string, args map[string]any) (any, bool) {
	if args == nil {
		args = map[string]any{}
	}

	switch name {
	case "list_posts":
		return h.toolListPosts(c, args)
	case "get_post":
		return h.toolGetPost(c, args)
	case "create_post":
		return h.toolCreatePost(c, args)
	case "update_post":
		return h.toolUpdatePost(c, args)
	case "delete_post":
		return h.toolDeletePost(c, args)
	case "reschedule_post":
		return h.toolReschedulePost(c, args)
	case "retry_post":
		return h.toolRetryPost(c, args)
	case "list_accounts":
		return h.toolListAccounts(c, args)
	case "list_footers":
		return h.toolListFooters(c, args)
	case "create_footer":
		return h.toolCreateFooter(c, args)
	case "update_footer":
		return h.toolUpdateFooter(c, args)
	case "delete_footer":
		return h.toolDeleteFooter(c, args)
	case "list_mentions":
		return h.toolListMentions(c, args)
	case "create_mention":
		return h.toolCreateMention(c, args)
	case "update_mention":
		return h.toolUpdateMention(c, args)
	case "delete_mention":
		return h.toolDeleteMention(c, args)
	case "list_watermarks":
		return h.toolListWatermarks(c, args)
	case "delete_watermark":
		return h.toolDeleteWatermark(c, args)
	case "get_profile":
		return h.toolGetProfile(c, args)
	case "create_news_draft":
		return h.toolCreateNewsDraft(c, args)
	case "list_news_drafts":
		return h.toolListNewsDrafts(c, args)
	case "get_news_draft":
		return h.toolGetNewsDraft(c, args)
	case "update_news_draft":
		return h.toolUpdateNewsDraft(c, args)
	case "delete_news_draft":
		return h.toolDeleteNewsDraft(c, args)
	case "post_news_draft":
		return h.toolPostNewsDraft(c, args)
	case "create_episode_draft":
		return h.toolCreateEpisodeDraft(c, args)
	case "list_episode_drafts":
		return h.toolListEpisodeDrafts(c, args)
	case "get_episode_draft":
		return h.toolGetEpisodeDraft(c, args)
	case "update_episode_draft":
		return h.toolUpdateEpisodeDraft(c, args)
	case "delete_episode_draft":
		return h.toolDeleteEpisodeDraft(c, args)
	case "post_episode_draft":
		return h.toolPostEpisodeDraft(c, args)
	case "upload_image":
		return h.toolUploadImage(c, args)
	case "create_upload_url":
		return h.toolCreateUploadURL(c, args)
	case "get_post_insights":
		return h.toolGetPostInsights(c, args)
	case "get_account_insights":
		return h.toolGetAccountInsights(c, args)
	case "update_watermark":
		return h.toolUpdateWatermark(c, args)
	case "list_media":
		return h.toolListMedia(c, args)
	case "add_media":
		return h.toolAddMedia(c, args)
	case "update_media":
		return h.toolUpdateMedia(c, args)
	case "delete_media":
		return h.toolDeleteMedia(c, args)
	case "get_image":
		return h.toolGetImage(c, args)
	case "compose_image":
		return h.toolComposeImage(c, args)
	case "capture_url":
		return h.toolCaptureURL(c, args)
	case "rewrite_instagram":
		return h.toolRewriteInstagram(c, args)
	case "check_instagram_caption":
		return h.toolCheckInstagramCaption(c, args)
	case "find_existing":
		return h.toolFindExisting(c, args)
	case "get_draft_queue":
		return h.toolGetDraftQueue(c, args)
	case "import_bgg":
		return h.toolImportBGG(c, args)
	case "upload_from_url":
		return h.toolUploadFromURL(c, args)
	case "get_growth_settings":
		return h.toolGetGrowthSettings(c, args)
	case "get_open_slots":
		return h.toolGetOpenSlots(c, args)
	case "transcribe":
		return h.toolTranscribe(c, args)
	default:
		return map[string]string{"error": "Unknown tool: " + name}, true
	}
}

// thumbOffsetArg reads a reel's cover frame (ms) from the thumbOffset
// argument; nil when absent or negative.
func thumbOffsetArg(args map[string]any) *int {
	v, ok := args["thumbOffset"].(float64)
	if !ok {
		return nil
	}
	o := int(v)
	return cleanThumbOffset(&o)
}

// designsArg reads the designs argument of create_post/update_post: nil when
// absent.
func designsArg(args map[string]any) (*[]postDesignInput, error) {
	v, ok := args["designs"]
	if !ok || v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("designs: %v", err)
	}
	var in []postDesignInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("designs must be a list of {design, imageUrl?, file?, previewUrl?}: %v", err)
	}
	return &in, nil
}

func strArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func strSliceArg(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mapStrArg(args map[string]any, key string) map[string]string {
	v, ok := args[key]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}

// --- Post tools ---

func (h *MCPHandler) toolListPosts(c *gin.Context, args map[string]any) (any, bool) {
	filter := mcpScopeFilter(c)

	if status := strArg(args, "status"); status != "" {
		filter["status"] = status
	}
	if platform := strArg(args, "platform"); platform != "" {
		filter["platforms"] = platform
	}
	if origin := strArg(args, "origin"); origin != "" {
		filter["origin"] = origin
	}
	if start := strArg(args, "start"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			if filter["scheduledAt"] == nil {
				filter["scheduledAt"] = bson.M{}
			}
			filter["scheduledAt"].(bson.M)["$gte"] = t
		}
	}
	if end := strArg(args, "end"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			if filter["scheduledAt"] == nil {
				filter["scheduledAt"] = bson.M{}
			}
			filter["scheduledAt"].(bson.M)["$lte"] = t
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The designs' JSON is left out (get_post has it); a list of them would
	// fill the context.
	opts := options.Find().SetSort(bson.D{{Key: "scheduledAt", Value: 1}}).SetProjection(bson.M{"designs.design": 0})
	cursor, err := h.DB.Posts().Find(ctx, filter, opts)
	if err != nil {
		return map[string]string{"error": "Failed to fetch posts"}, true
	}
	defer cursor.Close(ctx)

	var posts []models.Post
	if err := cursor.All(ctx, &posts); err != nil {
		return map[string]string{"error": "Failed to decode posts"}, true
	}
	if posts == nil {
		posts = []models.Post{}
	}
	return posts, false
}

func (h *MCPHandler) toolGetPost(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	postID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid post ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = postID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var post models.Post
	if err := h.DB.Posts().FindOne(ctx, filter).Decode(&post); err != nil {
		return map[string]string{"error": "Post not found"}, true
	}
	return post, false
}

func (h *MCPHandler) toolCreatePost(c *gin.Context, args map[string]any) (any, bool) {
	content := strArg(args, "content")
	platforms := strSliceArg(args, "platforms")
	scheduledAtStr := strArg(args, "scheduledAt")

	if len(platforms) == 0 {
		return map[string]string{"error": "platforms is required"}, true
	}
	if scheduledAtStr == "" {
		return map[string]string{"error": "scheduledAt is required"}, true
	}

	scheduledAt, err := time.Parse(time.RFC3339, scheduledAtStr)
	if err != nil {
		return map[string]string{"error": "Invalid scheduledAt format, use RFC3339"}, true
	}

	platModels := make([]models.Platform, len(platforms))
	for i, p := range platforms {
		platModels[i] = models.Platform(p)
	}

	status := models.PostStatusScheduled
	if s := strArg(args, "status"); s != "" {
		status = models.PostStatus(s)
	}

	postType := models.PostTypePost
	if pt := strArg(args, "postType"); pt != "" {
		postType = models.PostType(pt)
	}

	if status == models.PostStatusDraft {
		if err := h.checkDraftCap(c); err != nil {
			return map[string]string{"error": err.Error()}, true
		}
	}

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	designsIn, err := designsArg(args)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	imageURLs := strSliceArg(args, "imageUrls")
	uploaded, err := h.processInlineImages(args)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	imageURLs = append(imageURLs, uploaded...)
	var designs []models.PostDesign
	var designURLs []string
	if designsIn != nil {
		designs, designURLs, err = buildPostDesigns(*designsIn, uploaded, imageURLs)
		if err == nil && status != models.PostStatusDraft && hasUnexportedDesign(designs) {
			err = errUnexportedDesign
		}
		if err != nil {
			cleanupUploads(h.DB, h.UploadDir, uploaded)
			return map[string]string{"error": err.Error()}, true
		}
	}

	post := models.Post{
		UserID:           objID,
		PostType:         postType,
		Content:          content,
		FirstComment:     strArg(args, "firstComment"),
		Platforms:        platModels,
		ScheduledAt:      scheduledAt,
		Status:           status,
		Tags:             strSliceArg(args, "tags"),
		AccountIDs:       mapStrArg(args, "accountIds"),
		FooterIDs:        mapStrArg(args, "footerIds"),
		ContentOverrides: mapStrArg(args, "contentOverrides"),
		ImageURLs:        imageURLs,
		Collaborators:    cleanUsernames(strSliceArg(args, "collaborators"), 3),
		UserTags:         cleanUsernames(strSliceArg(args, "userTags"), 20),
		CoverURL:         strings.TrimSpace(strArg(args, "coverUrl")),
		ThumbOffset:      thumbOffsetArg(args),
		SourceURL:        strings.TrimSpace(strArg(args, "sourceUrl")),
		SourceKey:        sourceKey(strArg(args, "sourceUrl")),
		Origin:           mcpOrigin(args),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if len(designs) > 0 {
		post.Designs, post.DesignURLs = designs, designURLs
	}

	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		post.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.Posts().InsertOne(ctx, post)
	if err != nil {
		return map[string]string{"error": "Failed to create post"}, true
	}
	post.ID = result.InsertedID.(primitive.ObjectID)
	return post, false
}

func (h *MCPHandler) toolUpdatePost(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	postID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid post ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = postID

	update := bson.M{"updatedAt": time.Now()}

	if v, ok := args["content"]; ok {
		if s, ok := v.(string); ok {
			update["content"] = s
		}
	}
	if v, ok := args["firstComment"]; ok {
		if s, ok := v.(string); ok {
			update["firstComment"] = s
		}
	}
	if v, ok := args["postType"]; ok {
		if s, ok := v.(string); ok {
			update["postType"] = s
		}
	}
	if platforms := strSliceArg(args, "platforms"); platforms != nil {
		platModels := make([]models.Platform, len(platforms))
		for i, p := range platforms {
			platModels[i] = models.Platform(p)
		}
		update["platforms"] = platModels
	}
	if scheduledAt := strArg(args, "scheduledAt"); scheduledAt != "" {
		if t, err := time.Parse(time.RFC3339, scheduledAt); err == nil {
			update["scheduledAt"] = t
		}
	}
	if tags := strSliceArg(args, "tags"); tags != nil {
		update["tags"] = tags
	}
	if v, ok := args["status"]; ok {
		if s, ok := v.(string); ok {
			update["status"] = s
		}
	}
	if accountIDs := mapStrArg(args, "accountIds"); accountIDs != nil {
		update["accountIds"] = accountIDs
	}
	if footerIDs := mapStrArg(args, "footerIds"); footerIDs != nil {
		update["footerIds"] = footerIDs
	}
	if overrides := mapStrArg(args, "contentOverrides"); overrides != nil {
		update["contentOverrides"] = overrides
	}
	if v, ok := args["sourceUrl"].(string); ok {
		update["sourceUrl"] = strings.TrimSpace(v)
		update["sourceKey"] = sourceKey(v)
	}
	if _, ok := args["collaborators"]; ok {
		update["collaborators"] = cleanUsernames(strSliceArg(args, "collaborators"), 3)
	}
	if _, ok := args["userTags"]; ok {
		update["userTags"] = cleanUsernames(strSliceArg(args, "userTags"), 20)
	}
	unset := bson.M{}
	if v, ok := args["coverUrl"].(string); ok {
		if v = strings.TrimSpace(v); v != "" {
			update["coverUrl"] = v
		} else {
			unset["coverUrl"] = ""
		}
	}
	if _, ok := args["thumbOffset"]; ok {
		if o := thumbOffsetArg(args); o != nil {
			update["thumbOffset"] = *o
		} else {
			unset["thumbOffset"] = ""
		}
	}
	designs, err := designsArg(args)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	imageURLs := strSliceArg(args, "imageUrls")
	uploaded, err := h.processInlineImages(args)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	if len(uploaded) > 0 {
		if imageURLs == nil {
			imageURLs = uploaded
		} else {
			imageURLs = append(imageURLs, uploaded...)
		}
	}
	if imageURLs != nil {
		update["imageUrls"] = imageURLs
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var status *models.PostStatus
	if s, ok := update["status"].(string); ok {
		st := models.PostStatus(s)
		status = &st
	}
	if err := updatePostDesigns(ctx, h.DB.Posts(), filter, designs, uploaded, imageURLs, status, update, unset); err != nil {
		cleanupUploads(h.DB, h.UploadDir, uploaded)
		return map[string]string{"error": err.Error()}, true
	}

	previousImages := previousUploadURLs(ctx, h.DB.Posts(), filter, update)
	if previousImages == nil && unsetsUploads(unset) {
		previousImages = collectUploadURLs(ctx, h.DB.Posts(), filter)
	}
	change := bson.M{"$set": update}
	if len(unset) > 0 {
		change["$unset"] = unset
	}

	res, err := h.DB.Posts().UpdateOne(ctx, filter, change)
	if err != nil || res.MatchedCount == 0 {
		return map[string]string{"error": "Post not found"}, true
	}
	cleanupUploads(h.DB, h.UploadDir, previousImages)

	var post models.Post
	h.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	return post, false
}

func (h *MCPHandler) toolDeletePost(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	postID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid post ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = postID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	urls, err := deleteOneAndCollectUploadURLs(ctx, h.DB.Posts(), filter)
	if err != nil {
		return map[string]string{"error": "Post not found"}, true
	}
	cleanupUploads(h.DB, h.UploadDir, urls)
	return map[string]string{"message": "Post deleted"}, false
}

func (h *MCPHandler) toolReschedulePost(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	scheduledAtStr := strArg(args, "scheduledAt")
	if id == "" || scheduledAtStr == "" {
		return map[string]string{"error": "id and scheduledAt are required"}, true
	}

	postID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid post ID"}, true
	}
	scheduledAt, err := time.Parse(time.RFC3339, scheduledAtStr)
	if err != nil {
		return map[string]string{"error": "Invalid date format, use RFC3339"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = postID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := h.DB.Posts().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{"scheduledAt": scheduledAt, "updatedAt": time.Now()},
	})
	if err != nil || res.MatchedCount == 0 {
		return map[string]string{"error": "Post not found"}, true
	}

	var post models.Post
	h.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	return post, false
}

func (h *MCPHandler) toolRetryPost(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	postID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid post ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = postID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var post models.Post
	if err := h.DB.Posts().FindOne(ctx, filter).Decode(&post); err != nil {
		return map[string]string{"error": "Post not found"}, true
	}
	if post.Status != models.PostStatusFailed {
		return map[string]string{"error": "Only failed posts can be retried"}, true
	}

	h.DB.Posts().UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{
			"status":      models.PostStatusScheduled,
			"scheduledAt": time.Now(),
			"results":     nil,
			"updatedAt":   time.Now(),
		},
	})

	h.DB.Posts().FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	return post, false
}

// --- Account tools ---

func (h *MCPHandler) toolListAccounts(c *gin.Context, _ map[string]any) (any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"isActive": true}
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		filter["teamId"] = tid
	}

	cursor, err := h.DB.SocialAccounts().Find(ctx, filter)
	if err != nil {
		return map[string]string{"error": "Failed to fetch accounts"}, true
	}
	defer cursor.Close(ctx)

	var accounts []models.SocialAccount
	if err := cursor.All(ctx, &accounts); err != nil {
		return map[string]string{"error": "Failed to decode accounts"}, true
	}
	if accounts == nil {
		accounts = []models.SocialAccount{}
	}
	return accounts, false
}

// --- Footer tools ---

func (h *MCPHandler) toolListFooters(c *gin.Context, _ map[string]any) (any, bool) {
	filter := mcpScopeFilter(c)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := h.DB.Footers().Find(ctx, filter, opts)
	if err != nil {
		return map[string]string{"error": "Failed to fetch footers"}, true
	}
	defer cursor.Close(ctx)

	var footers []models.Footer
	if err := cursor.All(ctx, &footers); err != nil {
		return map[string]string{"error": "Failed to decode footers"}, true
	}
	if footers == nil {
		footers = []models.Footer{}
	}
	return footers, false
}

func (h *MCPHandler) toolCreateFooter(c *gin.Context, args map[string]any) (any, bool) {
	name := strArg(args, "name")
	content := strArg(args, "content")
	if name == "" || content == "" {
		return map[string]string{"error": "name and content are required"}, true
	}

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	footer := models.Footer{
		UserID:    objID,
		Name:      name,
		Content:   content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		footer.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.DB.Footers().InsertOne(ctx, footer)
	if err != nil {
		return map[string]string{"error": "Failed to create footer"}, true
	}
	footer.ID = result.InsertedID.(primitive.ObjectID)
	return footer, false
}

func (h *MCPHandler) toolUpdateFooter(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	footerID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid footer ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = footerID

	update := bson.M{"updatedAt": time.Now()}
	if v, ok := args["name"]; ok {
		if s, ok := v.(string); ok {
			update["name"] = s
		}
	}
	if v, ok := args["content"]; ok {
		if s, ok := v.(string); ok {
			update["content"] = s
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := h.DB.Footers().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || res.MatchedCount == 0 {
		return map[string]string{"error": "Footer not found"}, true
	}

	var footer models.Footer
	h.DB.Footers().FindOne(ctx, bson.M{"_id": footerID}).Decode(&footer)
	return footer, false
}

func (h *MCPHandler) toolDeleteFooter(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	footerID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid footer ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = footerID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := h.DB.Footers().DeleteOne(ctx, filter)
	if err != nil || res.DeletedCount == 0 {
		return map[string]string{"error": "Footer not found"}, true
	}
	return map[string]string{"message": "Footer deleted"}, false
}

// --- Mention tools ---

func (h *MCPHandler) toolListMentions(c *gin.Context, _ map[string]any) (any, bool) {
	filter := mcpScopeFilter(c)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := h.DB.Mentions().Find(ctx, filter, opts)
	if err != nil {
		return map[string]string{"error": "Failed to fetch mentions"}, true
	}
	defer cursor.Close(ctx)

	var mentions []models.MentionEntry
	if err := cursor.All(ctx, &mentions); err != nil {
		return map[string]string{"error": "Failed to decode mentions"}, true
	}
	if mentions == nil {
		mentions = []models.MentionEntry{}
	}
	return mentions, false
}

func (h *MCPHandler) toolCreateMention(c *gin.Context, args map[string]any) (any, bool) {
	name := strArg(args, "name")
	if name == "" {
		return map[string]string{"error": "name is required"}, true
	}

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	handles := mapStrArg(args, "handles")
	if handles == nil {
		handles = map[string]string{}
	}

	mention := models.MentionEntry{
		UserID:    objID,
		Name:      name,
		Country:   strArg(args, "country"),
		Handles:   handles,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		mention.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := h.DB.Mentions().InsertOne(ctx, mention)
	if err != nil {
		return map[string]string{"error": "Failed to create mention"}, true
	}
	mention.ID = result.InsertedID.(primitive.ObjectID)
	return mention, false
}

func (h *MCPHandler) toolUpdateMention(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	mentionID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid mention ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = mentionID

	update := bson.M{"updatedAt": time.Now()}
	if v, ok := args["name"]; ok {
		if s, ok := v.(string); ok {
			update["name"] = s
		}
	}
	if v, ok := args["country"]; ok {
		if s, ok := v.(string); ok {
			update["country"] = s
		}
	}
	if handles := mapStrArg(args, "handles"); handles != nil {
		update["handles"] = handles
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := h.DB.Mentions().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || res.MatchedCount == 0 {
		return map[string]string{"error": "Mention not found"}, true
	}

	var mention models.MentionEntry
	h.DB.Mentions().FindOne(ctx, bson.M{"_id": mentionID}).Decode(&mention)
	return mention, false
}

func (h *MCPHandler) toolDeleteMention(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	mentionID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid mention ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = mentionID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := h.DB.Mentions().DeleteOne(ctx, filter)
	if err != nil || res.DeletedCount == 0 {
		return map[string]string{"error": "Mention not found"}, true
	}
	return map[string]string{"message": "Mention deleted"}, false
}

// --- Watermark tools ---

func (h *MCPHandler) toolDeleteWatermark(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	wmID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid watermark ID"}, true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := deleteWatermark(ctx, h.DB, h.UploadDir, mcpScopeFilter(c), wmID); err != nil {
		return map[string]string{"error": "Watermark not found"}, true
	}
	return map[string]string{"message": "Watermark deleted"}, false
}

// --- Profile tool ---

func (h *MCPHandler) toolGetProfile(c *gin.Context, _ map[string]any) (any, bool) {
	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if isTeam, ok := c.Get("isTeamToken"); ok && isTeam.(bool) {
		var team models.Team
		if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": objID}).Decode(&team); err != nil {
			return map[string]string{"error": "Team not found"}, true
		}
		return map[string]any{
			"id":       team.ID,
			"name":     team.Name,
			"isAdmin":  false,
			"teamId":   team.ID,
			"teamName": team.Name,
		}, false
	}

	var user models.User
	if err := h.DB.Users().FindOne(ctx, bson.M{"_id": objID}).Decode(&user); err != nil {
		return map[string]string{"error": "User not found"}, true
	}

	// The MCP server exposes user features only, so the session is never an
	// admin one here, whatever the account's role in the web app.
	profile := map[string]any{
		"id":          user.ID,
		"email":       user.Email,
		"name":        user.Name,
		"isAdmin":     false,
		"isTeamAdmin": false,
		"teamId":      user.TeamID,
		"createdAt":   user.CreatedAt,
		"updatedAt":   user.UpdatedAt,
	}
	if user.TeamID != nil {
		var team models.Team
		if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": *user.TeamID}).Decode(&team); err == nil {
			profile["teamName"] = team.Name
		}
	}
	return profile, false
}

// --- News draft tools ---

func boolArg(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (h *MCPHandler) toolCreateNewsDraft(c *gin.Context, args map[string]any) (any, bool) {
	if err := h.checkDraftCap(c); err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	platforms := strSliceArg(args, "platforms")
	platModels := make([]models.Platform, len(platforms))
	for i, p := range platforms {
		platModels[i] = models.Platform(p)
	}

	imageURLs := strSliceArg(args, "imageUrls")
	log.Printf("[MCP] toolCreateNewsDraft: imageUrls from args = %v", imageURLs)
	if uploaded, err := h.processInlineImages(args); err != nil {
		log.Printf("[MCP] toolCreateNewsDraft: processInlineImages error: %v", err)
		return map[string]string{"error": err.Error()}, true
	} else {
		log.Printf("[MCP] toolCreateNewsDraft: processInlineImages returned %d urls: %v", len(uploaded), uploaded)
		imageURLs = append(imageURLs, uploaded...)
	}

	autoFetched := false
	if len(imageURLs) == 0 {
		if articleURL := strArg(args, "articleUrl"); articleURL != "" {
			log.Printf("[MCP] toolCreateNewsDraft: no images provided, fetching from articleUrl %s", articleURL)
			if fetched := h.fetchArticleImage(c, articleURL); len(fetched) > 0 {
				imageURLs = append(imageURLs, fetched...)
				autoFetched = true
			}
		}
	}

	if !autoFetched {
		if overlay := h.loadNewsOverlay(c); overlay != nil {
			log.Printf("[MCP] toolCreateNewsDraft: applying overlay to %d images", len(imageURLs))
			imageURLs = h.applyOverlayToImages(overlay, imageURLs)
		}
	}
	log.Printf("[MCP] toolCreateNewsDraft: final imageURLs = %v", imageURLs)

	draft := models.NewsDraft{
		UserID:           objID,
		EpisodeNumber:    strArg(args, "episodeNumber"),
		NewsTagline:      strArg(args, "newsTagline"),
		ArticleURL:       strArg(args, "articleUrl"),
		Shownotes:        strArg(args, "shownotes"),
		ImageURLs:        imageURLs,
		AddSocialPosting: boolArg(args, "addSocialPosting"),
		Content:          strArg(args, "content"),
		Platforms:        platModels,
		ScheduledAt:      strArg(args, "scheduledAt"),
		Tags:             strSliceArg(args, "tags"),
		Status:           models.PostStatus(strArg(args, "status")),
		FooterIDs:        mapStrArg(args, "footerIds"),
		ContentOverrides: mapStrArg(args, "contentOverrides"),
		AccountIDs:       mapStrArg(args, "accountIds"),
		FirstComment:     strArg(args, "firstComment"),
		PostType:         models.PostType(strArg(args, "postType")),
		SourceKey:        sourceKey(strArg(args, "articleUrl")),
		Origin:           mcpOrigin(args),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		draft.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.NewsDrafts().InsertOne(ctx, draft)
	if err != nil {
		return map[string]string{"error": "Failed to create draft"}, true
	}
	draft.ID = result.InsertedID.(primitive.ObjectID)
	return draft, false
}

func (h *MCPHandler) toolListNewsDrafts(c *gin.Context, args map[string]any) (any, bool) {
	filter := mcpScopeFilter(c)
	posted, _ := args["posted"].(bool)
	applyPostedFilter(filter, posted)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(postedSort(posted))
	cursor, err := h.DB.NewsDrafts().Find(ctx, filter, opts)
	if err != nil {
		return map[string]string{"error": "Failed to fetch drafts"}, true
	}
	defer cursor.Close(ctx)

	var drafts []models.NewsDraft
	if err := cursor.All(ctx, &drafts); err != nil {
		return map[string]string{"error": "Failed to decode drafts"}, true
	}
	if drafts == nil {
		drafts = []models.NewsDraft{}
	}
	return drafts, false
}

func (h *MCPHandler) toolGetNewsDraft(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	draftID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid draft ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = draftID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var draft models.NewsDraft
	if err := h.DB.NewsDrafts().FindOne(ctx, filter).Decode(&draft); err != nil {
		return map[string]string{"error": "Draft not found"}, true
	}
	return draft, false
}

func (h *MCPHandler) toolUpdateNewsDraft(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	draftID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid draft ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = draftID

	update := bson.M{"updatedAt": time.Now()}

	if v, ok := args["episodeNumber"]; ok {
		if s, ok := v.(string); ok {
			update["episodeNumber"] = s
		}
	}
	if v, ok := args["newsTagline"]; ok {
		if s, ok := v.(string); ok {
			update["newsTagline"] = s
		}
	}
	if v, ok := args["articleUrl"]; ok {
		if s, ok := v.(string); ok {
			update["articleUrl"] = s
			update["sourceKey"] = sourceKey(s)
		}
	}
	if v, ok := args["shownotes"]; ok {
		if s, ok := v.(string); ok {
			update["shownotes"] = s
		}
	}
	if v, ok := args["content"]; ok {
		if s, ok := v.(string); ok {
			update["content"] = s
		}
	}
	if v, ok := args["firstComment"]; ok {
		if s, ok := v.(string); ok {
			update["firstComment"] = s
		}
	}
	if v, ok := args["scheduledAt"]; ok {
		if s, ok := v.(string); ok {
			update["scheduledAt"] = s
		}
	}
	if v, ok := args["addSocialPosting"]; ok {
		if b, ok := v.(bool); ok {
			update["addSocialPosting"] = b
		}
	}
	if v, ok := args["postType"]; ok {
		if s, ok := v.(string); ok {
			update["postType"] = s
		}
	}
	if v, ok := args["status"]; ok {
		if s, ok := v.(string); ok {
			update["status"] = s
		}
	}
	if platforms := strSliceArg(args, "platforms"); platforms != nil {
		platModels := make([]models.Platform, len(platforms))
		for i, p := range platforms {
			platModels[i] = models.Platform(p)
		}
		update["platforms"] = platModels
	}
	if tags := strSliceArg(args, "tags"); tags != nil {
		update["tags"] = tags
	}
	imageURLs := strSliceArg(args, "imageUrls")
	if uploaded, err := h.processInlineImages(args); err != nil {
		return map[string]string{"error": err.Error()}, true
	} else if len(uploaded) > 0 {
		if overlay := h.loadNewsOverlay(c); overlay != nil {
			uploaded = h.applyOverlayToImages(overlay, uploaded)
		}
		if imageURLs == nil {
			imageURLs = uploaded
		} else {
			imageURLs = append(imageURLs, uploaded...)
		}
	}
	if imageURLs != nil {
		update["imageUrls"] = imageURLs
	}
	if footerIDs := mapStrArg(args, "footerIds"); footerIDs != nil {
		update["footerIds"] = footerIDs
	}
	if overrides := mapStrArg(args, "contentOverrides"); overrides != nil {
		update["contentOverrides"] = overrides
	}
	if accountIDs := mapStrArg(args, "accountIds"); accountIDs != nil {
		update["accountIds"] = accountIDs
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	previousImages := previousUploadURLs(ctx, h.DB.NewsDrafts(), filter, update)

	res, err := h.DB.NewsDrafts().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || res.MatchedCount == 0 {
		return map[string]string{"error": "Draft not found"}, true
	}
	cleanupUploads(h.DB, h.UploadDir, previousImages)

	var draft models.NewsDraft
	h.DB.NewsDrafts().FindOne(ctx, bson.M{"_id": draftID}).Decode(&draft)
	return draft, false
}

func (h *MCPHandler) toolDeleteNewsDraft(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	draftID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid draft ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = draftID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	urls, err := deleteOneAndCollectUploadURLs(ctx, h.DB.NewsDrafts(), filter)
	if err != nil {
		return map[string]string{"error": "Draft not found"}, true
	}
	cleanupUploads(h.DB, h.UploadDir, urls)
	return map[string]string{"message": "Draft deleted"}, false
}

func (h *MCPHandler) toolPostNewsDraft(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	draftID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid draft ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = draftID

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var draft models.NewsDraft
	if err := h.DB.NewsDrafts().FindOne(ctx, filter).Decode(&draft); err != nil {
		return map[string]string{"error": "Draft not found"}, true
	}

	if draft.EpisodeNumber == "" || draft.NewsTagline == "" || draft.ArticleURL == "" {
		return map[string]string{"error": "Draft is incomplete: episodeNumber, newsTagline, and articleUrl are required"}, true
	}

	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		return map[string]string{"error": "No team associated with this account"}, true
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		return map[string]string{"error": "Invalid team ID"}, true
	}

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		return map[string]string{"error": "Team not found"}, true
	}

	pluginEnabled := false
	for _, p := range team.EnabledPlugins {
		if p == "news_creator" {
			pluginEnabled = true
			break
		}
	}
	if !pluginEnabled {
		return map[string]string{"error": "news_creator plugin is not enabled for this team"}, true
	}
	if team.NewsCreatorURL == "" || team.NewsCreatorBearerToken == "" {
		return map[string]string{"error": "News creator URL or bearer token not configured"}, true
	}

	nh := &NewsHandler{DB: h.DB, UploadDir: h.UploadDir}
	input := &NewsSubmitInput{
		EpisodeNumber:    draft.EpisodeNumber,
		NewsTagline:      draft.NewsTagline,
		ArticleURL:       draft.ArticleURL,
		Shownotes:        draft.Shownotes,
		AddSocialPosting: draft.AddSocialPosting,
		Content:          draft.Content,
		Platforms:        draft.Platforms,
		ScheduledAt:      draft.ScheduledAt,
		ImageURLs:        draft.ImageURLs,
		Tags:             draft.Tags,
		Status:           draft.Status,
		FooterIDs:        draft.FooterIDs,
		ContentOverrides: draft.ContentOverrides,
		AccountIDs:       draft.AccountIDs,
		FirstComment:     draft.FirstComment,
		PostType:         draft.PostType,
	}

	if newsErr := nh.sendToN8N(ctx, &team, input, draft.ImageURLs); newsErr != nil {
		return map[string]string{"error": "Failed to send news: " + newsErr.Error()}, true
	}

	var post *models.Post
	if draft.AddSocialPosting {
		created, postErr := nh.createPost(ctx, c, input, nil)
		if postErr != nil {
			return map[string]string{"error": "News sent but failed to create post: " + postErr.Error()}, true
		}
		post = created
	}

	if err := markDraftPosted(ctx, h.DB.NewsDrafts(), bson.M{"_id": draftID}, nil); err != nil {
		log.Printf("[NewsCreator] Warning: could not mark draft %s as posted: %v", draftID.Hex(), err)
	}

	resp := map[string]any{"message": "News submitted successfully"}
	if post != nil {
		resp["post"] = post
	}
	return resp, false
}

// --- Episode draft tools ---

func (h *MCPHandler) toolCreateEpisodeDraft(c *gin.Context, args map[string]any) (any, bool) {
	if err := h.checkDraftCap(c); err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	platforms := strSliceArg(args, "platforms")
	platModels := make([]models.Platform, len(platforms))
	for i, p := range platforms {
		platModels[i] = models.Platform(p)
	}

	imageURLs := strSliceArg(args, "imageUrls")
	if uploaded, err := h.processInlineImages(args); err != nil {
		return map[string]string{"error": err.Error()}, true
	} else {
		imageURLs = append(imageURLs, uploaded...)
	}

	episodeType := strArg(args, "episodeType")
	if overlay := h.loadEpisodeOverlay(c, episodeType); overlay != nil {
		imageURLs = h.applyOverlayToImages(overlay, imageURLs)
	}

	draft := models.EpisodeDraft{
		UserID:            objID,
		EpisodeNumber:     strArg(args, "episodeNumber"),
		EpisodeTitle:      strArg(args, "episodeTitle"),
		EpisodeType:       episodeType,
		Summary:           strArg(args, "summary"),
		EpisodeDate:       strArg(args, "episodeDate"),
		GameNamePublisher: strArg(args, "gameNamePublisher"),
		LinkPublisher:     strArg(args, "linkPublisher"),
		LinkBGG:           strArg(args, "linkBGG"),
		Rules:             strArg(args, "rules"),
		Scene:             strArg(args, "scene"),
		IntroText:         strArg(args, "introText"),
		ImageURLs:         imageURLs,
		AddSocialPosting:  boolArg(args, "addSocialPosting"),
		Content:           strArg(args, "content"),
		Platforms:         platModels,
		ScheduledAt:       strArg(args, "scheduledAt"),
		Tags:              strSliceArg(args, "tags"),
		Status:            models.PostStatus(strArg(args, "status")),
		FooterIDs:         mapStrArg(args, "footerIds"),
		ContentOverrides:  mapStrArg(args, "contentOverrides"),
		AccountIDs:        mapStrArg(args, "accountIds"),
		FirstComment:      strArg(args, "firstComment"),
		PostType:          models.PostType(strArg(args, "postType")),
		Origin:            mcpOrigin(args),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if teamID, ok := c.Get("teamId"); ok {
		tid, _ := primitive.ObjectIDFromHex(teamID.(string))
		draft.TeamID = &tid
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.DB.EpisodeDrafts().InsertOne(ctx, draft)
	if err != nil {
		return map[string]string{"error": "Failed to create draft"}, true
	}
	draft.ID = result.InsertedID.(primitive.ObjectID)
	return draft, false
}

func (h *MCPHandler) toolListEpisodeDrafts(c *gin.Context, args map[string]any) (any, bool) {
	filter := mcpScopeFilter(c)
	posted, _ := args["posted"].(bool)
	applyPostedFilter(filter, posted)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(postedSort(posted))
	cursor, err := h.DB.EpisodeDrafts().Find(ctx, filter, opts)
	if err != nil {
		return map[string]string{"error": "Failed to fetch drafts"}, true
	}
	defer cursor.Close(ctx)

	var drafts []models.EpisodeDraft
	if err := cursor.All(ctx, &drafts); err != nil {
		return map[string]string{"error": "Failed to decode drafts"}, true
	}
	if drafts == nil {
		drafts = []models.EpisodeDraft{}
	}
	return drafts, false
}

func (h *MCPHandler) toolGetEpisodeDraft(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	draftID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid draft ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = draftID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var draft models.EpisodeDraft
	if err := h.DB.EpisodeDrafts().FindOne(ctx, filter).Decode(&draft); err != nil {
		return map[string]string{"error": "Draft not found"}, true
	}
	return draft, false
}

func (h *MCPHandler) toolUpdateEpisodeDraft(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	draftID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid draft ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = draftID

	update := bson.M{"updatedAt": time.Now()}

	if v, ok := args["episodeNumber"]; ok {
		if s, ok := v.(string); ok {
			update["episodeNumber"] = s
		}
	}
	if v, ok := args["episodeTitle"]; ok {
		if s, ok := v.(string); ok {
			update["episodeTitle"] = s
		}
	}
	if v, ok := args["episodeType"]; ok {
		if s, ok := v.(string); ok {
			update["episodeType"] = s
		}
	}
	if v, ok := args["summary"]; ok {
		if s, ok := v.(string); ok {
			update["summary"] = s
		}
	}
	if v, ok := args["episodeDate"]; ok {
		if s, ok := v.(string); ok {
			update["episodeDate"] = s
		}
	}
	if v, ok := args["gameNamePublisher"]; ok {
		if s, ok := v.(string); ok {
			update["gameNamePublisher"] = s
		}
	}
	if v, ok := args["linkPublisher"]; ok {
		if s, ok := v.(string); ok {
			update["linkPublisher"] = s
		}
	}
	if v, ok := args["linkBGG"]; ok {
		if s, ok := v.(string); ok {
			update["linkBGG"] = s
		}
	}
	if v, ok := args["rules"]; ok {
		if s, ok := v.(string); ok {
			update["rules"] = s
		}
	}
	if v, ok := args["scene"]; ok {
		if s, ok := v.(string); ok {
			update["scene"] = s
		}
	}
	if v, ok := args["introText"]; ok {
		if s, ok := v.(string); ok {
			update["introText"] = s
		}
	}
	if v, ok := args["content"]; ok {
		if s, ok := v.(string); ok {
			update["content"] = s
		}
	}
	if v, ok := args["firstComment"]; ok {
		if s, ok := v.(string); ok {
			update["firstComment"] = s
		}
	}
	if v, ok := args["scheduledAt"]; ok {
		if s, ok := v.(string); ok {
			update["scheduledAt"] = s
		}
	}
	if v, ok := args["addSocialPosting"]; ok {
		if b, ok := v.(bool); ok {
			update["addSocialPosting"] = b
		}
	}
	if v, ok := args["postType"]; ok {
		if s, ok := v.(string); ok {
			update["postType"] = s
		}
	}
	if v, ok := args["status"]; ok {
		if s, ok := v.(string); ok {
			update["status"] = s
		}
	}
	if platforms := strSliceArg(args, "platforms"); platforms != nil {
		platModels := make([]models.Platform, len(platforms))
		for i, p := range platforms {
			platModels[i] = models.Platform(p)
		}
		update["platforms"] = platModels
	}
	if tags := strSliceArg(args, "tags"); tags != nil {
		update["tags"] = tags
	}
	imageURLs := strSliceArg(args, "imageUrls")
	if uploaded, err := h.processInlineImages(args); err != nil {
		return map[string]string{"error": err.Error()}, true
	} else if len(uploaded) > 0 {
		episodeType := strArg(args, "episodeType")
		if episodeType == "" {
			readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer readCancel()
			var existing models.EpisodeDraft
			if err := h.DB.EpisodeDrafts().FindOne(readCtx, filter).Decode(&existing); err == nil {
				episodeType = existing.EpisodeType
			}
		}
		if overlay := h.loadEpisodeOverlay(c, episodeType); overlay != nil {
			uploaded = h.applyOverlayToImages(overlay, uploaded)
		}
		if imageURLs == nil {
			imageURLs = uploaded
		} else {
			imageURLs = append(imageURLs, uploaded...)
		}
	}
	if imageURLs != nil {
		update["imageUrls"] = imageURLs
	}
	if footerIDs := mapStrArg(args, "footerIds"); footerIDs != nil {
		update["footerIds"] = footerIDs
	}
	if overrides := mapStrArg(args, "contentOverrides"); overrides != nil {
		update["contentOverrides"] = overrides
	}
	if accountIDs := mapStrArg(args, "accountIds"); accountIDs != nil {
		update["accountIds"] = accountIDs
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	previousImages := previousUploadURLs(ctx, h.DB.EpisodeDrafts(), filter, update)

	res, err := h.DB.EpisodeDrafts().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || res.MatchedCount == 0 {
		return map[string]string{"error": "Draft not found"}, true
	}
	cleanupUploads(h.DB, h.UploadDir, previousImages)

	var draft models.EpisodeDraft
	h.DB.EpisodeDrafts().FindOne(ctx, bson.M{"_id": draftID}).Decode(&draft)
	return draft, false
}

func (h *MCPHandler) toolDeleteEpisodeDraft(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	draftID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid draft ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = draftID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	urls, err := deleteOneAndCollectUploadURLs(ctx, h.DB.EpisodeDrafts(), filter)
	if err != nil {
		return map[string]string{"error": "Draft not found"}, true
	}
	cleanupUploads(h.DB, h.UploadDir, urls)
	return map[string]string{"message": "Draft deleted"}, false
}

func (h *MCPHandler) toolPostEpisodeDraft(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	draftID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid draft ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = draftID

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var draft models.EpisodeDraft
	if err := h.DB.EpisodeDrafts().FindOne(ctx, filter).Decode(&draft); err != nil {
		return map[string]string{"error": "Draft not found"}, true
	}

	if draft.EpisodeNumber == "" || draft.EpisodeTitle == "" || draft.EpisodeType == "" || draft.EpisodeDate == "" {
		return map[string]string{"error": "Draft is incomplete: episodeNumber, episodeTitle, episodeType, and episodeDate are required"}, true
	}

	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		return map[string]string{"error": "No team associated with this account"}, true
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		return map[string]string{"error": "Invalid team ID"}, true
	}

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		return map[string]string{"error": "Team not found"}, true
	}

	pluginEnabled := false
	for _, p := range team.EnabledPlugins {
		if p == "episode_creator" {
			pluginEnabled = true
			break
		}
	}
	if !pluginEnabled {
		return map[string]string{"error": "episode_creator plugin is not enabled for this team"}, true
	}
	if team.EpisodeCreatorURL == "" || team.EpisodeCreatorBearerToken == "" {
		return map[string]string{"error": "Episode creator URL or bearer token not configured"}, true
	}

	eh := &EpisodeHandler{DB: h.DB, UploadDir: h.UploadDir}
	input := &EpisodeSubmitInput{
		EpisodeNumber:     draft.EpisodeNumber,
		EpisodeTitle:      draft.EpisodeTitle,
		EpisodeType:       draft.EpisodeType,
		Summary:           draft.Summary,
		EpisodeDate:       draft.EpisodeDate,
		GameNamePublisher: draft.GameNamePublisher,
		LinkPublisher:     draft.LinkPublisher,
		LinkBGG:           draft.LinkBGG,
		Rules:             draft.Rules,
		Scene:             draft.Scene,
		IntroText:         draft.IntroText,
		AddSocialPosting:  draft.AddSocialPosting,
		Content:           draft.Content,
		Platforms:         draft.Platforms,
		ScheduledAt:       draft.ScheduledAt,
		ImageURLs:         draft.ImageURLs,
		Tags:              draft.Tags,
		Status:            draft.Status,
		FooterIDs:         draft.FooterIDs,
		ContentOverrides:  draft.ContentOverrides,
		AccountIDs:        draft.AccountIDs,
		FirstComment:      draft.FirstComment,
		PostType:          draft.PostType,
	}

	if webhookErr := eh.sendToWebhook(ctx, &team, input, draft.ImageURLs); webhookErr != nil {
		return map[string]string{"error": "Failed to send episode: " + webhookErr.Error()}, true
	}

	var post *models.Post
	if draft.AddSocialPosting {
		created, postErr := eh.createPost(ctx, c, input, nil)
		if postErr != nil {
			return map[string]string{"error": "Episode sent but failed to create post: " + postErr.Error()}, true
		}
		post = created
	}

	if err := markDraftPosted(ctx, h.DB.EpisodeDrafts(), bson.M{"_id": draftID}, nil); err != nil {
		log.Printf("[EpisodeCreator] Warning: could not mark draft %s as posted: %v", draftID.Hex(), err)
	}

	resp := map[string]any{"message": "Episode submitted successfully"}
	if post != nil {
		resp["post"] = post
	}
	return resp, false
}

// --- Episode/news overlay helpers ---

func (h *MCPHandler) loadEpisodeOverlay(c *gin.Context, episodeType string) image.Image {
	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		return nil
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		return nil
	}

	var overlayID *primitive.ObjectID
	switch episodeType {
	case "news":
		overlayID = team.EpisodeOverlayNewsID
	case "review":
		overlayID = team.EpisodeOverlayReviewID
	case "special":
		overlayID = team.EpisodeOverlaySpecialID
	case "tatort":
		overlayID = team.EpisodeOverlayTatortID
	}
	if overlayID == nil {
		return nil
	}

	wm, err := findMediaItem(ctx, h.DB, *overlayID)
	if err != nil {
		return nil
	}

	wmData, _, err := uploadstore.Read(ctx, h.DB, h.UploadDir, filepath.Base(wm.URL))
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(wmData))
	if err != nil {
		return nil
	}
	return img
}

func (h *MCPHandler) loadNewsOverlay(c *gin.Context) image.Image {
	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		return nil
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		return nil
	}

	if team.NewsCreatorWatermarkID == nil {
		return nil
	}

	wm, err := findMediaItem(ctx, h.DB, *team.NewsCreatorWatermarkID)
	if err != nil {
		return nil
	}

	wmData, _, err := uploadstore.Read(ctx, h.DB, h.UploadDir, filepath.Base(wm.URL))
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(wmData))
	if err != nil {
		return nil
	}
	return img
}

func (h *MCPHandler) applyOverlayToImages(overlay image.Image, imageURLs []string) []string {
	if overlay == nil || len(imageURLs) == 0 {
		return imageURLs
	}
	result := make([]string, 0, len(imageURLs))
	for _, imgURL := range imageURLs {
		result = append(result, h.applyOverlayToSingleImage(overlay, imgURL))
	}
	return result
}

func (h *MCPHandler) applyOverlayToSingleImage(overlay image.Image, imgURL string) string {
	ext := strings.ToLower(filepath.Ext(imgURL))
	if ext == ".mp4" || ext == ".mov" {
		return imgURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	data, _, err := uploadstore.Read(ctx, h.DB, h.UploadDir, filepath.Base(imgURL))
	if err != nil {
		return imgURL
	}

	overlaid, _ := applyWatermarkOverlay(data, overlay)

	url, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("%d.jpg", time.Now().UnixNano()), "image/jpeg", overlaid)
	if err != nil {
		return imgURL
	}
	return url
}

// fetchArticleImage fetches the best content image from the given article URL,
// downloads it, optionally applies the news overlay, and returns the resulting
// image URLs. For BGG URLs it uses the BGG API. For other URLs it extracts
// image candidates (og:image, twitter:image, JSON-LD, <img> tag scoring) and
// tries them in order until one downloads successfully.
func (h *MCPHandler) fetchArticleImage(c *gin.Context, articleURL string) []string {
	if articleURL == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// BGG URLs use the dedicated BGG API
	if gameID := bggGameIDFromURL(articleURL); gameID != "" {
		var bggToken string
		var settings models.AppSettings
		if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err == nil {
			bggToken = settings.BGGAPIToken
		}
		_, bggImageURL, _ := fetchBGGPageInfo(ctx, gameID, articleURL, bggToken)
		if bggImageURL == "" {
			bggImageURL = fetchBGGCoverImageURL(ctx, articleURL)
		}
		if bggImageURL != "" && h.BGG != nil {
			log.Printf("[MCP] fetchArticleImage: downloading BGG image %s", bggImageURL)
			imgData, err := h.BGG.downloadAndProcess(ctx, c, bggImageURL, "news_entry")
			if err != nil {
				log.Printf("[MCP] fetchArticleImage: failed to download/process BGG image: %v", err)
				return nil
			}
			url, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("%d.jpg", time.Now().UnixNano()), "image/jpeg", imgData)
			if err != nil {
				log.Printf("[MCP] storing the image: %v", err)
				return nil
			}
			return []string{url}
		}
	}

	// Non-BGG: fetch the page and extract image candidates
	html, err := fetchPageHTML(ctx, articleURL)
	if err != nil {
		log.Printf("[MCP] fetchArticleImage: failed to fetch page HTML: %v", err)
		return nil
	}

	candidates := extractContentImageCandidates(html, articleURL)
	log.Printf("[MCP] fetchArticleImage: found %d image candidates for %s", len(candidates), articleURL)
	for i, c := range candidates {
		if i < 5 {
			log.Printf("[MCP] fetchArticleImage:   candidate %d: %s", i, c)
		}
	}

	if len(candidates) == 0 {
		log.Printf("[MCP] fetchArticleImage: no image found for URL %s", articleURL)
		return nil
	}

	overlay := h.loadNewsOverlay(c)

	// Try each candidate in order until one downloads successfully
	for _, imageURL := range candidates {
		if result := h.tryDownloadContentImage(ctx, c, imageURL, overlay); result != nil {
			return result
		}
	}

	log.Printf("[MCP] fetchArticleImage: all %d candidates failed to download for %s", len(candidates), articleURL)
	return nil
}

func (h *MCPHandler) tryDownloadContentImage(ctx context.Context, c *gin.Context, imageURL string, overlay image.Image) []string {
	if h.BGG != nil && overlay != nil {
		log.Printf("[MCP] tryDownloadContentImage: downloading with overlay: %s", imageURL)
		imgData, err := h.BGG.downloadAndProcessURL(ctx, c, imageURL, "news_entry")
		if err != nil {
			log.Printf("[MCP] tryDownloadContentImage: failed with overlay: %v", err)
			return nil
		}
		url, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("%d.jpg", time.Now().UnixNano()), "image/jpeg", imgData)
		if err != nil {
			log.Printf("[MCP] storing the image: %v", err)
			return nil
		}
		return []string{url}
	}

	log.Printf("[MCP] tryDownloadContentImage: downloading plain: %s", imageURL)
	saved, err := downloadAndSaveImage(ctx, h.DB, h.UploadDir, imageURL)
	if err != nil {
		log.Printf("[MCP] tryDownloadContentImage: failed plain download: %v", err)
		return nil
	}
	urls := []string{saved}
	if overlay != nil {
		urls = h.applyOverlayToImages(overlay, urls)
	}
	return urls
}

// --- Image upload tools ---

var mcpAllowedExtensions = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".png": "image/png", ".gif": "image/gif", ".webp": "image/webp",
	".mp4": "video/mp4", ".mov": "video/quicktime",
}

func cleanBase64(raw string) string {
	// Strip data URI prefix (e.g. "data:image/jpeg;base64,...")
	if idx := strings.Index(raw, ";base64,"); idx != -1 {
		raw = raw[idx+len(";base64,"):]
	} else if idx := strings.Index(raw, ","); idx != -1 && idx < 64 && strings.HasPrefix(raw, "data:") {
		raw = raw[idx+1:]
	}
	// Remove any whitespace / newlines
	raw = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, raw)
	// Convert URL-safe base64 characters to standard
	raw = strings.NewReplacer("-", "+", "_", "/").Replace(raw)
	// Fix missing padding
	if m := len(raw) % 4; m != 0 {
		raw += strings.Repeat("=", 4-m)
	}
	return raw
}

func (h *MCPHandler) saveBase64Upload(b64Data, filename string) (string, error) {
	b64Data = cleanBase64(b64Data)
	data, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", fmt.Errorf("invalid base64 data: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	ct, ok := mcpAllowedExtensions[ext]
	if !ok {
		return "", fmt.Errorf("unsupported file type %s (allowed: jpg, jpeg, png, gif, webp, mp4, mov)", ext)
	}

	maxSize := int64(10 * 1024 * 1024)
	if ext == ".mp4" || ext == ".mov" {
		maxSize = 100 * 1024 * 1024
	}
	if int64(len(data)) > maxSize {
		return "", fmt.Errorf("file too large (max %dMB)", maxSize/(1024*1024))
	}

	storedName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return storeUploadBytes(ctx, h.DB, h.UploadDir, storedName, ct, data)
}

func (h *MCPHandler) processInlineImages(args map[string]any) ([]string, error) {
	v, ok := args["images"]
	if !ok {
		log.Printf("[MCP] processInlineImages: no 'images' key in args")
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		log.Printf("[MCP] processInlineImages: 'images' is not []any, got %T", v)
		return nil, fmt.Errorf("images must be an array")
	}
	log.Printf("[MCP] processInlineImages: processing %d images", len(arr))
	var urls []string
	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			log.Printf("[MCP] processInlineImages: images[%d] is not map[string]any, got %T", i, item)
			return nil, fmt.Errorf("images[%d] must be an object with data and filename", i)
		}
		data, _ := obj["data"].(string)
		filename, _ := obj["filename"].(string)
		log.Printf("[MCP] processInlineImages: images[%d] filename=%q dataLen=%d", i, filename, len(data))
		if data == "" || filename == "" {
			return nil, fmt.Errorf("images[%d]: both data and filename are required", i)
		}
		url, err := h.saveBase64Upload(data, filename)
		if err != nil {
			log.Printf("[MCP] processInlineImages: images[%d] saveBase64Upload error: %v", i, err)
			return nil, fmt.Errorf("images[%d]: %w", i, err)
		}
		log.Printf("[MCP] processInlineImages: images[%d] saved as %s", i, url)
		urls = append(urls, url)
	}
	log.Printf("[MCP] processInlineImages: returning %d urls: %v", len(urls), urls)
	return urls, nil
}

func (h *MCPHandler) toolUploadImage(_ *gin.Context, args map[string]any) (any, bool) {
	data := strArg(args, "data")
	filename := strArg(args, "filename")
	if data == "" || filename == "" {
		return map[string]string{"error": "data and filename are required"}, true
	}
	url, err := h.saveBase64Upload(data, filename)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	return map[string]string{"url": url, "filename": filepath.Base(url)}, false
}

// --- Tool definitions ---

func (h *MCPHandler) toolDefinitions() []mcpTool {
	tools := append(h.baseToolDefinitions(), growthToolDefinitions()...)
	tools = append(tools, agentToolDefinitions()...)
	tools = append(tools, agentQueueToolDefinitions()...)
	tools = append(tools, textToolDefinitions()...)
	tools = append(tools, transcribeToolDefinitions()...)
	return append(tools, imageToolDefinitions()...)
}

func (h *MCPHandler) baseToolDefinitions() []mcpTool {
	return []mcpTool{
		{
			Name:        "list_posts",
			Description: "List posts with optional filters for status, platform, and date range.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status":   map[string]any{"type": "string", "enum": []string{"draft", "scheduled", "published", "failed"}, "description": "Filter by post status"},
					"platform": map[string]any{"type": "string", "enum": []string{"bluesky", "instagram", "twitter", "mastodon", "threads", "linkedin", "youtube"}, "description": "Filter by platform"},
					"origin":   map[string]any{"type": "string", "description": "Only posts with this origin (e.g. mcp, agent)"},
					"start":    map[string]any{"type": "string", "description": "Start date filter (RFC3339 format)"},
					"end":      map[string]any{"type": "string", "description": "End date filter (RFC3339 format)"},
				},
			},
		},
		{
			Name:        "get_post",
			Description: "Get a single post by its ID.",
			InputSchema: map[string]any{
				"type":     "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Post ID"}},
				"required": []string{"id"},
			},
		},
		{
			Name:        "create_post",
			Description: "Create a new social media post. The post will be scheduled for the specified time.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content":          map[string]any{"type": "string", "description": "Post text content"},
					"platforms":        map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"bluesky", "instagram", "twitter", "mastodon", "threads", "linkedin", "youtube"}}, "description": "Target platforms"},
					"scheduledAt":      map[string]any{"type": "string", "description": "When to publish (RFC3339 format, e.g. 2025-01-15T14:00:00Z)"},
					"status":           map[string]any{"type": "string", "enum": []string{"draft", "scheduled"}, "description": "Post status (default: scheduled)"},
					"tags":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags for organizing posts"},
					"postType":         map[string]any{"type": "string", "enum": []string{"post", "story", "reel"}, "description": "Post type (default: post)"},
					"firstComment":     map[string]any{"type": "string", "description": "First comment text (Instagram)"},
					"accountIds":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Map of platform to specific account ID to use"},
					"footerIds":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Map of platform to footer ID to append"},
					"contentOverrides": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Map of platform to platform-specific content override"},
					"sourceUrl":        map[string]any{"type": "string", "description": "The page or BGG entry the post is about; find_existing uses it to detect duplicates"},
					"origin":           map[string]any{"type": "string", "description": "Short label for who created it, e.g. agent (lower-case slug; default mcp). Drafts with an origin count towards the team's agent draft limit and are marked in the Drafts list."},
					"collaborators":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Instagram usernames to invite as collaborators (max 3); the post also appears on their profile once they accept"},
					"userTags":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Instagram usernames to tag on the image (max 20)"},
					"coverUrl":         map[string]any{"type": "string", "description": "Reel only: URL of an uploaded image to use as the reel's cover"},
					"thumbOffset":      map[string]any{"type": "integer", "description": "Reel only: the video frame (milliseconds from the start) to use as the cover when there is no coverUrl"},
					"designs": map[string]any{"type": "array", "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"design":     map[string]any{"type": "object", "description": "The image editor design: a template like compose_image's media library templates ({version: 1, width, height, background, layers, and for a video duration in seconds, audio and the layers' anim}), or an editor draft ({version: 1, template, photo?: {src, crop, aspectId, adjustments}}). Image layers, clips and audio refer to uploaded files by URL (create_upload_url)."},
							"imageUrl":   map[string]any{"type": "string", "description": "The post image (one of its imageUrls) this design was exported as; leave out for a design not exported yet"},
							"file":       map[string]any{"type": "integer", "description": "Instead of imageUrl: the index of the export among this call's inline images"},
							"previewUrl": map[string]any{"type": "string", "description": "An uploaded still shown in place of a design not exported yet (e.g. from compose_image with the template)"},
						},
						"required": []string{"design"},
					}, "description": "Editable designs for the post's images or videos: the post editor opens them in the image editor as they were. A design without imageUrl/file has not been exported yet (a video, or an image too large to upload): the user exports it in the post editor, so such a post must be a draft. On update_post the list replaces the stored designs (empty clears); left out, they stay."},
					"imageUrls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "URLs of previously uploaded images"},
					"images": map[string]any{"type": "array", "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"data":     map[string]any{"type": "string", "description": "Base64-encoded image data"},
							"filename": map[string]any{"type": "string", "description": "Original filename with extension (e.g. photo.jpg)"},
						},
						"required": []string{"data", "filename"},
					}, "description": "Images to upload inline. Each object has base64-encoded data and a filename. Supported types: jpg, jpeg, png, gif, webp, mp4, mov."},
				},
				"required": []string{"platforms", "scheduledAt"},
			},
		},
		{
			Name:        "update_post",
			Description: "Update an existing post. Only provided fields are changed.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":               map[string]any{"type": "string", "description": "Post ID"},
					"content":          map[string]any{"type": "string", "description": "New post content"},
					"platforms":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "New target platforms"},
					"scheduledAt":      map[string]any{"type": "string", "description": "New scheduled time (RFC3339)"},
					"status":           map[string]any{"type": "string", "enum": []string{"draft", "scheduled"}, "description": "New status"},
					"tags":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "New tags"},
					"postType":         map[string]any{"type": "string", "description": "New post type"},
					"firstComment":     map[string]any{"type": "string", "description": "New first comment"},
					"accountIds":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-account ID map"},
					"footerIds":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-footer ID map"},
					"contentOverrides": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-specific content overrides"},
					"sourceUrl":        map[string]any{"type": "string", "description": "The page or BGG entry the post is about; find_existing uses it to detect duplicates"},
					"collaborators":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Instagram collaborator usernames (max 3); an empty list clears them"},
					"userTags":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Instagram usernames tagged on the image; an empty list clears them"},
					"coverUrl":         map[string]any{"type": "string", "description": "Reel only: URL of an uploaded cover image; an empty string clears it"},
					"thumbOffset":      map[string]any{"type": "integer", "description": "Reel only: the cover frame in milliseconds; -1 clears it"},
					"designs": map[string]any{"type": "array", "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"design":     map[string]any{"type": "object", "description": "The image editor design: a template like compose_image's media library templates ({version: 1, width, height, background, layers, and for a video duration in seconds, audio and the layers' anim}), or an editor draft ({version: 1, template, photo?: {src, crop, aspectId, adjustments}}). Image layers, clips and audio refer to uploaded files by URL (create_upload_url)."},
							"imageUrl":   map[string]any{"type": "string", "description": "The post image (one of its imageUrls) this design was exported as; leave out for a design not exported yet"},
							"file":       map[string]any{"type": "integer", "description": "Instead of imageUrl: the index of the export among this call's inline images"},
							"previewUrl": map[string]any{"type": "string", "description": "An uploaded still shown in place of a design not exported yet (e.g. from compose_image with the template)"},
						},
						"required": []string{"design"},
					}, "description": "Editable designs for the post's images or videos: the post editor opens them in the image editor as they were. A design without imageUrl/file has not been exported yet (a video, or an image too large to upload): the user exports it in the post editor, so such a post must be a draft. On update_post the list replaces the stored designs (empty clears); left out, they stay."},
					"imageUrls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "URLs of previously uploaded images"},
					"images": map[string]any{"type": "array", "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"data":     map[string]any{"type": "string", "description": "Base64-encoded image data"},
							"filename": map[string]any{"type": "string", "description": "Original filename with extension (e.g. photo.jpg)"},
						},
						"required": []string{"data", "filename"},
					}, "description": "Images to upload inline. Each object has base64-encoded data and a filename."},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "delete_post",
			Description: "Delete a post by its ID.",
			InputSchema: map[string]any{
				"type":     "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Post ID"}},
				"required": []string{"id"},
			},
		},
		{
			Name:        "reschedule_post",
			Description: "Reschedule a post to a different time.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string", "description": "Post ID"},
					"scheduledAt": map[string]any{"type": "string", "description": "New scheduled time (RFC3339 format)"},
				},
				"required": []string{"id", "scheduledAt"},
			},
		},
		{
			Name:        "retry_post",
			Description: "Retry publishing a failed post. Resets its status to scheduled for immediate publishing.",
			InputSchema: map[string]any{
				"type":     "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Post ID (must have status 'failed')"}},
				"required": []string{"id"},
			},
		},
		{
			Name:        "list_accounts",
			Description: "List all active social media accounts available for posting.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "list_footers",
			Description: "List all post footers. Footers are appended to posts at publish time.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "create_footer",
			Description: "Create a new post footer.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "description": "Footer name"},
					"content": map[string]any{"type": "string", "description": "Footer text content"},
				},
				"required": []string{"name", "content"},
			},
		},
		{
			Name:        "update_footer",
			Description: "Update an existing footer.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Footer ID"},
					"name":    map[string]any{"type": "string", "description": "New name"},
					"content": map[string]any{"type": "string", "description": "New content"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "delete_footer",
			Description: "Delete a footer by its ID.",
			InputSchema: map[string]any{
				"type":     "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Footer ID"}},
				"required": []string{"id"},
			},
		},
		{
			Name:        "list_mentions",
			Description: "List all mention entries. Mentions map names to platform-specific handles.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "create_mention",
			Description: "Create a new mention entry with platform-specific handles.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "description": "Display name"},
					"country": map[string]any{"type": "string", "description": "Country code"},
					"handles": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Map of platform to @handle (e.g. {\"bluesky\": \"@user.bsky.social\", \"twitter\": \"@user\"})"},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "update_mention",
			Description: "Update an existing mention entry.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "description": "Mention ID"},
					"name":    map[string]any{"type": "string", "description": "New name"},
					"country": map[string]any{"type": "string", "description": "New country code"},
					"handles": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "New handles map"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "delete_mention",
			Description: "Delete a mention entry by its ID.",
			InputSchema: map[string]any{
				"type":     "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Mention ID"}},
				"required": []string{"id"},
			},
		},
		{
			Name: "list_watermarks",
			Description: "List the team's watermarks (media library images marked as watermarks; the id also works as a mediaId) with their description, size, what the team uses each for (roles, e.g. BGG cover overlay) " +
				"and fullCanvas, which marks overlays meant to cover the whole image (frames, banners: use anchor full in compose_image) as opposed to small logos. " +
				"With preview=true a thumbnail of each (on grey) is attached.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"preview": map[string]any{"type": "boolean", "description": "Attach thumbnails"},
				},
			},
		},
		{
			Name:        "delete_watermark",
			Description: "Delete a watermark by its ID. This removes the image from the media library; to keep it as an ordinary image, unmark it with update_media instead.",
			InputSchema: map[string]any{
				"type":     "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Watermark ID"}},
				"required": []string{"id"},
			},
		},
		{
			Name:        "get_profile",
			Description: "Get the current user's profile information.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "create_news_draft",
			Description: "Create a news draft for later review and posting. Drafts store news data (episode number, tagline, article URL, shownotes) and optionally social media posting settings. The draft can be reviewed and submitted later via post_news_draft. If no images are provided and an articleUrl is given, the og:image is automatically fetched from the article page. If the team has a news creator watermark configured, it is automatically composited onto images.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"origin":           map[string]any{"type": "string", "description": "Short label for who created it, e.g. agent (lower-case slug; default mcp). Drafts with an origin count towards the team's agent draft limit and are marked in the Drafts list."},
					"episodeNumber":    map[string]any{"type": "string", "description": "Episode number"},
					"newsTagline":      map[string]any{"type": "string", "description": "Short tagline for the news episode"},
					"articleUrl":       map[string]any{"type": "string", "description": "URL of the news article"},
					"shownotes":        map[string]any{"type": "string", "description": "Additional show notes"},
					"imageUrls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "URLs of previously uploaded images"},
					"images": map[string]any{"type": "array", "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"data":     map[string]any{"type": "string", "description": "Base64-encoded image data"},
							"filename": map[string]any{"type": "string", "description": "Original filename with extension (e.g. photo.jpg)"},
						},
						"required": []string{"data", "filename"},
					}, "description": "Images to upload inline. Each object has base64-encoded data and a filename."},
					"addSocialPosting": map[string]any{"type": "boolean", "description": "Whether to create a social media post when the draft is submitted"},
					"content":          map[string]any{"type": "string", "description": "Social media post content"},
					"platforms":        map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"bluesky", "instagram", "twitter", "mastodon", "threads", "linkedin", "youtube"}}, "description": "Target platforms for the social post"},
					"scheduledAt":      map[string]any{"type": "string", "description": "When to publish the social post (RFC3339)"},
					"status":           map[string]any{"type": "string", "enum": []string{"draft", "scheduled"}, "description": "Status for the social post"},
					"tags":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags"},
					"postType":         map[string]any{"type": "string", "enum": []string{"post", "story", "reel"}, "description": "Post type (default: post)"},
					"firstComment":     map[string]any{"type": "string", "description": "First comment text"},
					"accountIds":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-account ID map"},
					"footerIds":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-footer ID map"},
					"contentOverrides": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-specific content overrides"},
				},
			},
		},
		{
			Name:        "list_news_drafts",
			Description: "List open news drafts, sorted by most recently updated. Set posted to true to list the drafts that were already submitted instead, newest first.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"posted": map[string]any{"type": "boolean", "description": "List already-submitted drafts instead of open ones"},
			}},
		},
		{
			Name:        "get_news_draft",
			Description: "Get a single news draft by its ID.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Draft ID"}},
				"required":   []string{"id"},
			},
		},
		{
			Name:        "update_news_draft",
			Description: "Update an existing news draft. Only provided fields are changed.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":               map[string]any{"type": "string", "description": "Draft ID"},
					"episodeNumber":    map[string]any{"type": "string", "description": "Episode number"},
					"newsTagline":      map[string]any{"type": "string", "description": "News tagline"},
					"articleUrl":       map[string]any{"type": "string", "description": "Article URL"},
					"shownotes":        map[string]any{"type": "string", "description": "Show notes"},
					"imageUrls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "URLs of previously uploaded images"},
					"images": map[string]any{"type": "array", "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"data":     map[string]any{"type": "string", "description": "Base64-encoded image data"},
							"filename": map[string]any{"type": "string", "description": "Original filename with extension (e.g. photo.jpg)"},
						},
						"required": []string{"data", "filename"},
					}, "description": "Images to upload inline. Each object has base64-encoded data and a filename."},
					"addSocialPosting": map[string]any{"type": "boolean", "description": "Whether to include a social media post"},
					"content":          map[string]any{"type": "string", "description": "Social media post content"},
					"platforms":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Target platforms"},
					"scheduledAt":      map[string]any{"type": "string", "description": "Scheduled time (RFC3339)"},
					"status":           map[string]any{"type": "string", "enum": []string{"draft", "scheduled"}, "description": "Social post status"},
					"tags":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags"},
					"postType":         map[string]any{"type": "string", "description": "Post type"},
					"firstComment":     map[string]any{"type": "string", "description": "First comment"},
					"accountIds":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-account ID map"},
					"footerIds":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-footer ID map"},
					"contentOverrides": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-specific content overrides"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "delete_news_draft",
			Description: "Delete a news draft by its ID.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Draft ID"}},
				"required":   []string{"id"},
			},
		},
		{
			Name:        "post_news_draft",
			Description: "Submit a news draft for publishing. This sends the news to the configured webhook and optionally creates a social media post, then marks the draft as posted. The draft is kept and can be listed again with list_news_drafts using posted=true. The draft must have episodeNumber, newsTagline, and articleUrl filled in.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Draft ID"}},
				"required":   []string{"id"},
			},
		},
		{
			Name:        "create_episode_draft",
			Description: "Create an episode draft for later review and posting. Drafts store episode data (number, title, type, date, review details) and optionally social media posting settings. The draft can be reviewed and submitted later via post_episode_draft. If the team has an overlay configured for the given episodeType (news/review/special/tatort), it is automatically composited onto uploaded images.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"origin":           map[string]any{"type": "string", "description": "Short label for who created it, e.g. agent (lower-case slug; default mcp). Drafts with an origin count towards the team's agent draft limit and are marked in the Drafts list."},
					"episodeNumber":     map[string]any{"type": "string", "description": "Episode number"},
					"episodeTitle":      map[string]any{"type": "string", "description": "Episode title"},
					"episodeType":       map[string]any{"type": "string", "enum": []string{"news", "review", "special", "tatort"}, "description": "Episode type"},
					"summary":           map[string]any{"type": "string", "description": "Episode summary"},
					"episodeDate":       map[string]any{"type": "string", "description": "Episode date (ISO datetime)"},
					"gameNamePublisher": map[string]any{"type": "string", "description": "Game name and publisher (review type)"},
					"linkPublisher":     map[string]any{"type": "string", "description": "Publisher link (review type)"},
					"linkBGG":           map[string]any{"type": "string", "description": "BoardGameGeek link (review type)"},
					"rules":             map[string]any{"type": "string", "description": "Rules text (review type)"},
					"scene":             map[string]any{"type": "string", "description": "Scene description (review type)"},
					"introText":         map[string]any{"type": "string", "description": "Introduction text (review type)"},
					"imageUrls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "URLs of previously uploaded images"},
					"images": map[string]any{"type": "array", "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"data":     map[string]any{"type": "string", "description": "Base64-encoded image data"},
							"filename": map[string]any{"type": "string", "description": "Original filename with extension (e.g. photo.jpg)"},
						},
						"required": []string{"data", "filename"},
					}, "description": "Images to upload inline. Each object has base64-encoded data and a filename."},
					"addSocialPosting": map[string]any{"type": "boolean", "description": "Whether to create a social media post when the draft is submitted"},
					"content":           map[string]any{"type": "string", "description": "Social media post content"},
					"platforms":         map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"bluesky", "instagram", "twitter", "mastodon", "threads", "linkedin", "youtube"}}, "description": "Target platforms for the social post"},
					"scheduledAt":       map[string]any{"type": "string", "description": "When to publish the social post (RFC3339)"},
					"status":            map[string]any{"type": "string", "enum": []string{"draft", "scheduled"}, "description": "Status for the social post"},
					"tags":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags"},
					"postType":          map[string]any{"type": "string", "enum": []string{"post", "story", "reel"}, "description": "Post type (default: post)"},
					"firstComment":      map[string]any{"type": "string", "description": "First comment text"},
					"accountIds":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-account ID map"},
					"footerIds":         map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-footer ID map"},
					"contentOverrides":  map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-specific content overrides"},
				},
			},
		},
		{
			Name:        "list_episode_drafts",
			Description: "List open episode drafts, sorted by most recently updated. Set posted to true to list the drafts that were already submitted instead, newest first.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"posted": map[string]any{"type": "boolean", "description": "List already-submitted drafts instead of open ones"},
			}},
		},
		{
			Name:        "get_episode_draft",
			Description: "Get a single episode draft by its ID.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Draft ID"}},
				"required":   []string{"id"},
			},
		},
		{
			Name:        "update_episode_draft",
			Description: "Update an existing episode draft. Only provided fields are changed.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":                map[string]any{"type": "string", "description": "Draft ID"},
					"episodeNumber":     map[string]any{"type": "string", "description": "Episode number"},
					"episodeTitle":      map[string]any{"type": "string", "description": "Episode title"},
					"episodeType":       map[string]any{"type": "string", "enum": []string{"news", "review", "special", "tatort"}, "description": "Episode type"},
					"summary":           map[string]any{"type": "string", "description": "Episode summary"},
					"episodeDate":       map[string]any{"type": "string", "description": "Episode date"},
					"gameNamePublisher": map[string]any{"type": "string", "description": "Game name and publisher"},
					"linkPublisher":     map[string]any{"type": "string", "description": "Publisher link"},
					"linkBGG":           map[string]any{"type": "string", "description": "BGG link"},
					"rules":             map[string]any{"type": "string", "description": "Rules text"},
					"scene":             map[string]any{"type": "string", "description": "Scene description"},
					"introText":         map[string]any{"type": "string", "description": "Introduction text"},
					"imageUrls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "URLs of previously uploaded images"},
					"images": map[string]any{"type": "array", "items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"data":     map[string]any{"type": "string", "description": "Base64-encoded image data"},
							"filename": map[string]any{"type": "string", "description": "Original filename with extension (e.g. photo.jpg)"},
						},
						"required": []string{"data", "filename"},
					}, "description": "Images to upload inline. Each object has base64-encoded data and a filename."},
					"addSocialPosting": map[string]any{"type": "boolean", "description": "Whether to include a social media post"},
					"content":          map[string]any{"type": "string", "description": "Social media post content"},
					"platforms":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Target platforms"},
					"scheduledAt":      map[string]any{"type": "string", "description": "Scheduled time (RFC3339)"},
					"status":           map[string]any{"type": "string", "enum": []string{"draft", "scheduled"}, "description": "Social post status"},
					"tags":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags"},
					"postType":          map[string]any{"type": "string", "description": "Post type"},
					"firstComment":      map[string]any{"type": "string", "description": "First comment"},
					"accountIds":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-account ID map"},
					"footerIds":         map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-to-footer ID map"},
					"contentOverrides":  map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Platform-specific content overrides"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "delete_episode_draft",
			Description: "Delete an episode draft by its ID.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Draft ID"}},
				"required":   []string{"id"},
			},
		},
		{
			Name:        "post_episode_draft",
			Description: "Submit an episode draft for publishing. This sends the episode to the configured webhook and optionally creates a social media post, then marks the draft as posted. The draft is kept and can be listed again with list_episode_drafts using posted=true. The draft must have episodeNumber, episodeTitle, episodeType, and episodeDate filled in.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Draft ID"}},
				"required":   []string{"id"},
			},
		},
		{
			Name: "create_upload_url",
			Description: "Get a single-use URL to upload a file to directly, instead of passing it base64 through a tool call: the way to upload anything larger than a small image, above all videos and sounds. " +
				"Send the file there yourself (curl -T file <uploadUrl>, or curl -F file=@file <uploadUrl>; no Authorization header) before it expires; the URL then stops working. " +
				"The answer already carries the file's url, which works once the upload is done: use it in imageUrls, a design's image layers (src) and audio, coverUrl, add_media, get_image or transcribe. " +
				"An upload nothing references is deleted after a day.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filename":         map[string]any{"type": "string", "description": "The file's name with extension, which sets its type: jpg, jpeg, png, gif, webp (max 30 MB), mp4, mov, webm (max 200 MB), mp3, m4a, aac, wav, ogg (max 50 MB), ttf, otf (max 10 MB)"},
					"expiresInMinutes": map[string]any{"type": "integer", "description": "How long the URL stays valid (default 60, max 1440)"},
				},
				"required": []string{"filename"},
			},
		},
		{
			Name:        "upload_image",
			Description: "Upload an image for use in posts or drafts. Returns the image URL to pass in imageUrls when creating or updating posts, news drafts, or episode drafts.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"data":     map[string]any{"type": "string", "description": "Base64-encoded file data"},
					"filename": map[string]any{"type": "string", "description": "Original filename with extension (e.g. photo.jpg). Supported types: jpg, jpeg, png, gif, webp, mp4, mov."},
				},
				"required": []string{"data", "filename"},
			},
		},
	}
}

// formatToolError is unused but reserved for future structured errors.
func formatToolError(msg string, args ...any) string {
	return fmt.Sprintf(msg, args...)
}
