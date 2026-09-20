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
	"os"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MCPHandler struct {
	DB        *database.MongoDB
	UploadDir string
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
	Type string `json:"type"`
	Text string `json:"text"`
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
		resp = h.handleInitialize(req.ID)
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

func (h *MCPHandler) handleInitialize(id any) jsonrpcResponse {
	return jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"protocolVersion": "2025-03-26",
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

	text, _ := json.Marshal(result)
	return jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": []toolResultContent{{Type: "text", Text: string(text)}},
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
	default:
		return map[string]string{"error": "Unknown tool: " + name}, true
	}
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

	opts := options.Find().SetSort(bson.D{{Key: "scheduledAt", Value: 1}})
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

	userID, _ := c.Get("userId")
	objID, _ := primitive.ObjectIDFromHex(userID.(string))

	imageURLs := strSliceArg(args, "imageUrls")
	if uploaded, err := h.processInlineImages(args); err != nil {
		return map[string]string{"error": err.Error()}, true
	} else {
		imageURLs = append(imageURLs, uploaded...)
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
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
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
	imageURLs := strSliceArg(args, "imageUrls")
	if uploaded, err := h.processInlineImages(args); err != nil {
		return map[string]string{"error": err.Error()}, true
	} else if len(uploaded) > 0 {
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

	res, err := h.DB.Posts().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || res.MatchedCount == 0 {
		return map[string]string{"error": "Post not found"}, true
	}

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

	res, err := h.DB.Posts().DeleteOne(ctx, filter)
	if err != nil || res.DeletedCount == 0 {
		return map[string]string{"error": "Post not found"}, true
	}
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

func (h *MCPHandler) toolListWatermarks(c *gin.Context, _ map[string]any) (any, bool) {
	filter := mcpScopeFilter(c)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := h.DB.Watermarks().Find(ctx, filter)
	if err != nil {
		return map[string]string{"error": "Failed to fetch watermarks"}, true
	}
	defer cursor.Close(ctx)

	var watermarks []models.Watermark
	if err := cursor.All(ctx, &watermarks); err != nil {
		return map[string]string{"error": "Failed to decode watermarks"}, true
	}
	if watermarks == nil {
		watermarks = []models.Watermark{}
	}
	return watermarks, false
}

func (h *MCPHandler) toolDeleteWatermark(c *gin.Context, args map[string]any) (any, bool) {
	id := strArg(args, "id")
	if id == "" {
		return map[string]string{"error": "id is required"}, true
	}
	wmID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return map[string]string{"error": "Invalid watermark ID"}, true
	}

	filter := mcpScopeFilter(c)
	filter["_id"] = wmID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := h.DB.Watermarks().DeleteOne(ctx, filter)
	if err != nil || res.DeletedCount == 0 {
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

	profile := map[string]any{
		"id":          user.ID,
		"email":       user.Email,
		"name":        user.Name,
		"isAdmin":     user.IsAdmin,
		"isTeamAdmin": user.IsTeamAdmin,
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

	if overlay := h.loadNewsOverlay(c); overlay != nil {
		log.Printf("[MCP] toolCreateNewsDraft: applying overlay to %d images", len(imageURLs))
		imageURLs = h.applyOverlayToImages(overlay, imageURLs)
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

func (h *MCPHandler) toolListNewsDrafts(c *gin.Context, _ map[string]any) (any, bool) {
	filter := mcpScopeFilter(c)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
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

	res, err := h.DB.NewsDrafts().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || res.MatchedCount == 0 {
		return map[string]string{"error": "Draft not found"}, true
	}

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

	res, err := h.DB.NewsDrafts().DeleteOne(ctx, filter)
	if err != nil || res.DeletedCount == 0 {
		return map[string]string{"error": "Draft not found"}, true
	}
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

	h.DB.NewsDrafts().DeleteOne(ctx, bson.M{"_id": draftID})

	resp := map[string]any{"message": "News submitted successfully"}
	if post != nil {
		resp["post"] = post
	}
	return resp, false
}

// --- Episode draft tools ---

func (h *MCPHandler) toolCreateEpisodeDraft(c *gin.Context, args map[string]any) (any, bool) {
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

func (h *MCPHandler) toolListEpisodeDrafts(c *gin.Context, _ map[string]any) (any, bool) {
	filter := mcpScopeFilter(c)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}})
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

	res, err := h.DB.EpisodeDrafts().UpdateOne(ctx, filter, bson.M{"$set": update})
	if err != nil || res.MatchedCount == 0 {
		return map[string]string{"error": "Draft not found"}, true
	}

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

	res, err := h.DB.EpisodeDrafts().DeleteOne(ctx, filter)
	if err != nil || res.DeletedCount == 0 {
		return map[string]string{"error": "Draft not found"}, true
	}
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

	h.DB.EpisodeDrafts().DeleteOne(ctx, bson.M{"_id": draftID})

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
	}
	if overlayID == nil {
		return nil
	}

	var wm models.Watermark
	if err := h.DB.Watermarks().FindOne(ctx, bson.M{"_id": *overlayID}).Decode(&wm); err != nil {
		return nil
	}

	filename := filepath.Base(wm.URL)
	wmData, err := os.ReadFile(filepath.Join(h.UploadDir, filename))
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

	var wm models.Watermark
	if err := h.DB.Watermarks().FindOne(ctx, bson.M{"_id": *team.NewsCreatorWatermarkID}).Decode(&wm); err != nil {
		return nil
	}

	filename := filepath.Base(wm.URL)
	wmData, err := os.ReadFile(filepath.Join(h.UploadDir, filename))
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

	filename := filepath.Base(imgURL)
	fullPath := filepath.Join(h.UploadDir, filename)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var upload models.Upload
		if err := h.DB.Uploads().FindOne(ctx, bson.M{"filename": filename}).Decode(&upload); err != nil || len(upload.Data) == 0 {
			return imgURL
		}
		data = upload.Data
	}

	overlaid, _ := applyWatermarkOverlay(data, overlay)

	newFilename := fmt.Sprintf("%d.jpg", time.Now().UnixNano())
	newPath := filepath.Join(h.UploadDir, newFilename)
	if err := os.MkdirAll(h.UploadDir, 0o755); err != nil {
		return imgURL
	}
	if err := os.WriteFile(newPath, overlaid, 0o644); err != nil {
		return imgURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.DB.Uploads().InsertOne(ctx, models.Upload{
		Filename:    newFilename,
		ContentType: "image/jpeg",
		Data:        overlaid,
		Size:        int64(len(overlaid)),
		CreatedAt:   time.Now(),
	})

	return "/api/uploads/" + newFilename
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

	if err := os.MkdirAll(h.UploadDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create upload directory: %w", err)
	}
	dst := filepath.Join(h.UploadDir, storedName)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.DB.Uploads().InsertOne(ctx, models.Upload{
		Filename:    storedName,
		ContentType: ct,
		Data:        data,
		Size:        int64(len(data)),
		CreatedAt:   time.Now(),
	})
	return "/api/uploads/" + storedName, nil
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
	return []mcpTool{
		{
			Name:        "list_posts",
			Description: "List posts with optional filters for status, platform, and date range.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status":   map[string]any{"type": "string", "enum": []string{"draft", "scheduled", "published", "failed"}, "description": "Filter by post status"},
					"platform": map[string]any{"type": "string", "enum": []string{"bluesky", "instagram", "twitter", "mastodon", "threads", "linkedin", "youtube"}, "description": "Filter by platform"},
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
			Name:        "list_watermarks",
			Description: "List all available watermark images.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "delete_watermark",
			Description: "Delete a watermark by its ID.",
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
			Description: "Create a news draft for later review and posting. Drafts store news data (episode number, tagline, article URL, shownotes) and optionally social media posting settings. The draft can be reviewed and submitted later via post_news_draft. If the team has a news creator watermark configured, it is automatically composited onto uploaded images.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
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
			Description: "List all news drafts, sorted by most recently updated.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
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
			Description: "Submit a news draft for publishing. This sends the news to the configured webhook and optionally creates a social media post, then deletes the draft. The draft must have episodeNumber, newsTagline, and articleUrl filled in.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Draft ID"}},
				"required":   []string{"id"},
			},
		},
		{
			Name:        "create_episode_draft",
			Description: "Create an episode draft for later review and posting. Drafts store episode data (number, title, type, date, review details) and optionally social media posting settings. The draft can be reviewed and submitted later via post_episode_draft. If the team has an overlay configured for the given episodeType (news/review/special), it is automatically composited onto uploaded images.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"episodeNumber":     map[string]any{"type": "string", "description": "Episode number"},
					"episodeTitle":      map[string]any{"type": "string", "description": "Episode title"},
					"episodeType":       map[string]any{"type": "string", "enum": []string{"news", "review", "special"}, "description": "Episode type"},
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
			Description: "List all episode drafts, sorted by most recently updated.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
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
					"episodeType":       map[string]any{"type": "string", "enum": []string{"news", "review", "special"}, "description": "Episode type"},
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
			Description: "Submit an episode draft for publishing. This sends the episode to the configured webhook and optionally creates a social media post, then deletes the draft. The draft must have episodeNumber, episodeTitle, episodeType, and episodeDate filled in.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Draft ID"}},
				"required":   []string{"id"},
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
