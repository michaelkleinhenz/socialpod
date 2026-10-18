package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"socialmedia/internal/backup"
	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Agents run a prompt on a schedule against the user's MCP tools, calling the
// model through the user's own OpenRouter key. The tools are the MCP server's
// (same names, same code, run in-process with the user's context keys and the
// admin flags cleared, exactly as MCPAuthRequired would), plus web_search and
// fetch_url, which the MCP server does not need because MCP clients bring
// their own.

const (
	defaultAgentMaxSteps = 30
	maxAgentMaxSteps     = 100
	agentRunTimeout      = 30 * time.Minute
	agentModelTimeout    = 5 * time.Minute
	agentRunRetention    = 30 * 24 * time.Hour
	// agentToolResultLimit caps what one tool result puts into the context.
	agentToolResultLimit = 40000
	// agentLogLimit caps what the run log keeps of arguments and results.
	agentLogLimit   = 4000
	maxAgentLogSize = 300
	// maxAgentImages caps the tool images passed to the model per turn.
	maxAgentImages = 4
)

// agentHTTPClient calls OpenRouter; tests swap its transport.
var agentHTTPClient = &http.Client{Timeout: agentModelTimeout}

// AgentHandler serves the Agents REST API and runs the agents.
type AgentHandler struct {
	DB  *database.MongoDB
	MCP *MCPHandler

	stop    chan struct{}
	mu      sync.Mutex
	cancels map[primitive.ObjectID]context.CancelFunc
}

// StartScheduler starts the loop that runs due agents every 30 seconds.
// Runs left marked running by a previous process are closed first.
func (h *AgentHandler) StartScheduler() {
	if h.stop != nil {
		return
	}
	h.recoverInterrupted("Interrupted by a server restart")
	h.stop = make(chan struct{})
	ticker := time.NewTicker(30 * time.Second)
	log.Println("Agent scheduler started")
	go func() {
		h.runDue()
		for {
			select {
			case <-ticker.C:
				h.runDue()
			case <-h.stop:
				ticker.Stop()
				return
			}
		}
	}()
}

// StopScheduler stops the loop and cancels the runs in progress.
func (h *AgentHandler) StopScheduler() {
	if h.stop != nil {
		close(h.stop)
		h.stop = nil
	}
	h.cancelAll()
}

// cancelAll cancels every run in progress in this process.
func (h *AgentHandler) cancelAll() {
	h.mu.Lock()
	for _, cancel := range h.cancels {
		cancel()
	}
	h.mu.Unlock()
}

// recoverInterrupted closes the runs left marked running, by a previous
// process or in a restored backup, and frees their agents.
func (h *AgentHandler) recoverInterrupted(reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now().UTC()
	h.DB.AgentRuns().UpdateMany(ctx, bson.M{"status": models.AgentRunRunning}, bson.M{"$set": bson.M{
		"status": models.AgentRunError, "error": reason, "finishedAt": now,
	}})
	h.DB.Agents().UpdateMany(ctx, bson.M{"running": true}, bson.M{
		"$set":   bson.M{"running": false, "lastStatus": models.AgentRunError, "lastError": reason},
		"$unset": bson.M{"runStartedAt": ""},
	})
}

// runDue claims every enabled agent whose next run is due and starts it.
// Claiming moves nextRunAt forward in the same update, so an agent is never
// started twice for one slot.
func (h *AgentHandler) runDue() {
	if backup.RestoreInProgress() {
		return // a backup restore is replacing the data
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	now := time.Now().UTC()
	cur, err := h.DB.Agents().Find(ctx, bson.M{
		"enabled":   true,
		"running":   bson.M{"$ne": true},
		"nextRunAt": bson.M{"$lte": now},
	})
	if err != nil {
		log.Printf("[agents] query due agents: %v", err)
		return
	}
	var due []models.Agent
	if err := cur.All(ctx, &due); err != nil {
		return
	}
	for _, a := range due {
		set := bson.M{"running": true, "runStartedAt": now}
		update := bson.M{"$set": set}
		if next := nextAgentRun(a.Schedule, now); next != nil {
			set["nextRunAt"] = *next
		} else {
			update["$unset"] = bson.M{"nextRunAt": ""}
		}
		var claimed models.Agent
		err := h.DB.Agents().FindOneAndUpdate(ctx,
			bson.M{"_id": a.ID, "running": bson.M{"$ne": true}, "nextRunAt": bson.M{"$lte": now}},
			update,
		).Decode(&claimed)
		if err != nil {
			continue
		}
		if _, err := h.start(claimed, "schedule"); err != nil {
			log.Printf("[agents] start %s: %v", a.ID.Hex(), err)
		}
	}
}

// claimManual marks an agent running for a run started by hand, leaving its
// schedule alone. It fails when the agent is already running.
func (h *AgentHandler) claimManual(ctx context.Context, filter bson.M) (models.Agent, error) {
	filter["running"] = bson.M{"$ne": true}
	var a models.Agent
	err := h.DB.Agents().FindOneAndUpdate(ctx, filter,
		bson.M{"$set": bson.M{"running": true, "runStartedAt": time.Now().UTC()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&a)
	return a, err
}

// start records a run for a claimed agent and executes it in the background.
func (h *AgentHandler) start(a models.Agent, trigger string) (primitive.ObjectID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now().UTC()
	run := models.AgentRun{
		ID:        primitive.NewObjectID(),
		AgentID:   a.ID,
		UserID:    a.UserID,
		Trigger:   trigger,
		Status:    models.AgentRunRunning,
		StartedAt: now,
		Steps:     []models.AgentRunStep{},
		ExpiresAt: now.Add(agentRunRetention),
	}
	if _, err := h.DB.AgentRuns().InsertOne(ctx, run); err != nil {
		h.finishAgent(a.ID, models.AgentRunError, "Could not record the run: "+err.Error())
		return primitive.NilObjectID, err
	}

	runCtx, runCancel := context.WithTimeout(context.Background(), agentRunTimeout)
	h.mu.Lock()
	if h.cancels == nil {
		h.cancels = map[primitive.ObjectID]context.CancelFunc{}
	}
	h.cancels[a.ID] = runCancel
	h.mu.Unlock()

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.cancels, a.ID)
			h.mu.Unlock()
			runCancel()
		}()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[agents] run %s panicked: %v", run.ID.Hex(), r)
				h.finishRun(run.ID, a.ID, models.AgentRunError, "", fmt.Sprintf("internal error: %v", r))
			}
		}()
		h.execute(runCtx, a, run.ID)
	}()
	return run.ID, nil
}

// cancelRun stops a run in progress. It reports whether one was running in
// this process.
func (h *AgentHandler) cancelRun(agentID primitive.ObjectID) bool {
	h.mu.Lock()
	cancel, ok := h.cancels[agentID]
	h.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (h *AgentHandler) finishAgent(agentID primitive.ObjectID, status, errMsg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.DB.Agents().UpdateByID(ctx, agentID, bson.M{
		"$set":   bson.M{"running": false, "lastRunAt": time.Now().UTC(), "lastStatus": status, "lastError": errMsg},
		"$unset": bson.M{"runStartedAt": ""},
	})
}

func (h *AgentHandler) finishRun(runID, agentID primitive.ObjectID, status, summary, errMsg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.DB.AgentRuns().UpdateByID(ctx, runID, bson.M{"$set": bson.M{
		"status": status, "summary": summary, "error": errMsg, "finishedAt": time.Now().UTC(),
	}})
	h.finishAgent(agentID, status, errMsg)
}

// agentRunLog appends steps and usage to a run as it goes, so the UI can
// follow a run in progress.
type agentRunLog struct {
	db    *database.MongoDB
	runID primitive.ObjectID
	steps int
}

func (l *agentRunLog) add(step models.AgentRunStep) {
	if l.steps >= maxAgentLogSize {
		return
	}
	l.steps++
	step.At = time.Now().UTC()
	step.Text = truncateText(step.Text, agentLogLimit)
	step.Args = truncateText(step.Args, agentLogLimit)
	step.Result = truncateText(step.Result, agentLogLimit)
	update := bson.M{"$push": bson.M{"steps": step}}
	if step.Kind == "tool" {
		update["$inc"] = bson.M{"toolCalls": 1}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	l.db.AgentRuns().UpdateByID(ctx, l.runID, update)
}

func (l *agentRunLog) usage(u agentUsage) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	l.db.AgentRuns().UpdateByID(ctx, l.runID, bson.M{"$inc": bson.M{
		"promptTokens": u.PromptTokens, "completionTokens": u.CompletionTokens, "cost": u.Cost,
	}})
}

// agentOrigin is the origin label of what an agent creates: "agent-" and a
// slug of its name, short enough for originRe.
func agentOrigin(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "agent"
	}
	o := "agent-" + slug
	if len(o) > 32 {
		o = strings.TrimRight(o[:32], "-")
	}
	return o
}

// agentCreatingTools are the tools that store an origin; the runner fills it
// in when the model leaves it out, so an agent's drafts show who made them.
var agentCreatingTools = map[string]bool{
	"create_post": true, "create_news_draft": true, "create_episode_draft": true,
	"capture_url": true, "import_bgg": true,
}

// agentToolContext builds the Gin context a tool call runs with: the same
// keys MCPAuthRequired sets for this user, admin flags cleared.
func agentToolContext(ctx context.Context, user models.User) *gin.Context {
	rc, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/api/mcp", nil).WithContext(ctx)
	if u, err := url.Parse(appBaseURL); err == nil && u.Host != "" {
		req.Host = u.Host
	}
	rc.Request = req
	rc.Set("userId", user.ID.Hex())
	if user.TeamID != nil {
		rc.Set("teamId", user.TeamID.Hex())
	}
	rc.Set("isAdmin", false)
	rc.Set("isTeamAdmin", false)
	rc.Set("isAgent", true)
	return rc
}

// agentSystemPrompt is what every agent run starts with: how it runs, the
// time, its tools, then the user's global prompt.
func agentSystemPrompt(a models.Agent, global string, now time.Time) string {
	tz := a.Schedule.Timezone
	if tz == "" {
		tz = defaultAgentTimezone
	}
	local := now
	if loc, err := time.LoadLocation(tz); err == nil {
		local = now.In(loc)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are %q, an autonomous agent working for a SocialPod user. SocialPod plans, schedules and publishes social media posts.\n\n", a.Name)
	b.WriteString("You run on a schedule. Nobody watches this run and nobody will answer questions: decide on your own, act through the tools and never wait for a confirmation.\n\n")
	fmt.Fprintf(&b, "Current time: %s (%s, timezone %s).\n\n", local.Format("Monday, 2 January 2006 15:04"), local.Format(time.RFC3339), tz)
	b.WriteString("Tools:\n")
	b.WriteString("- The SocialPod tools: the same ones the SocialPod MCP server offers, acting with this user's rights. Prompts written for MCP clients work here unchanged.\n")
	b.WriteString("- web_search: search the web for current information. Returns results with titles, URLs and short summaries.\n")
	b.WriteString("- fetch_url: read the text, title, description and main image of a web page.\n\n")
	fmt.Fprintf(&b, "Everything you create is labelled with the origin %q automatically. ", agentOrigin(a.Name))
	b.WriteString("Before you cover a story, game or link, check with find_existing that it has not been covered yet, and respect the draft limit get_draft_queue reports.\n\n")
	b.WriteString("When you are done, reply with a short summary of what you did and why, without calling a tool. That reply ends the run.")
	if g := strings.TrimSpace(global); g != "" {
		b.WriteString("\n\nInstructions for all agents of this user:\n")
		b.WriteString(g)
	}
	return b.String()
}

type agentUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Cost             float64 `json:"cost"`
}

type agentChatResponse struct {
	Choices []struct {
		Message      map[string]any `json:"message"`
		FinishReason string         `json:"finish_reason"`
	} `json:"choices"`
	Usage *agentUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// agentToolCall is one function call the model asked for.
type agentToolCall struct {
	ID   string
	Name string
	Args string
}

func parseToolCalls(msg map[string]any) []agentToolCall {
	raw, _ := msg["tool_calls"].([]any)
	var calls []agentToolCall
	for _, item := range raw {
		m, _ := item.(map[string]any)
		fn, _ := m["function"].(map[string]any)
		id, _ := m["id"].(string)
		name, _ := fn["name"].(string)
		args, _ := fn["arguments"].(string)
		if name != "" {
			calls = append(calls, agentToolCall{ID: id, Name: name, Args: args})
		}
	}
	return calls
}

// assistantMessage keeps what the model's reply must carry into the next
// request: content, the tool calls, and reasoning details, which some
// providers require back alongside tool calls.
func assistantMessage(msg map[string]any) map[string]any {
	out := map[string]any{"role": "assistant", "content": msg["content"]}
	if out["content"] == nil {
		out["content"] = ""
	}
	for _, k := range []string{"tool_calls", "reasoning_details"} {
		if v, ok := msg[k]; ok && v != nil {
			out[k] = v
		}
	}
	return out
}

func messageText(msg map[string]any) string {
	switch v := msg["content"].(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, p := range v {
			if m, ok := p.(map[string]any); ok {
				if s, ok := m["text"].(string); ok {
					parts = append(parts, s)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}
	return ""
}

// errImagesUnsupported is returned when the model refuses image input.
var errImagesUnsupported = errors.New("model does not take images")

// agentChat sends one chat completion with tools. Rate limits and server
// errors are retried twice.
func agentChat(ctx context.Context, key, model string, messages []map[string]any, tools []map[string]any) (*agentChatResponse, error) {
	reqBody, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
		"tools":    tools,
		"usage":    map[string]any{"include": true},
	})
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * 5 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Title", "SocialPod Agents")
		if appBaseURL != "" {
			req.Header.Set("HTTP-Referer", appBaseURL)
		}
		resp, err := agentHTTPClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = fmt.Errorf("failed to reach OpenRouter: %w", err)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("OpenRouter returned %d: %s", resp.StatusCode, truncateText(string(body), 500))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			msg := truncateText(string(body), 1000)
			if strings.Contains(strings.ToLower(msg), "image") {
				return nil, fmt.Errorf("%w: %s", errImagesUnsupported, msg)
			}
			return nil, fmt.Errorf("OpenRouter returned %d: %s", resp.StatusCode, msg)
		}
		var out agentChatResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("invalid response from OpenRouter")
		}
		if out.Error != nil {
			lastErr = fmt.Errorf("OpenRouter: %s", out.Error.Message)
			continue
		}
		if len(out.Choices) == 0 {
			lastErr = fmt.Errorf("OpenRouter returned no answer")
			continue
		}
		return &out, nil
	}
	return nil, lastErr
}

// hasImageParts reports whether any message carries an image.
func hasImageParts(messages []map[string]any) bool {
	for _, m := range messages {
		if parts, ok := m["content"].([]map[string]any); ok {
			for _, p := range parts {
				if p["type"] == "image_url" {
					return true
				}
			}
		}
	}
	return false
}

// withoutImageParts drops the image parts the runner added, keeping their
// text.
func withoutImageParts(messages []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		parts, ok := m["content"].([]map[string]any)
		if !ok {
			out = append(out, m)
			continue
		}
		var text []string
		for _, p := range parts {
			if s, ok := p["text"].(string); ok {
				text = append(text, s)
			}
		}
		out = append(out, map[string]any{"role": m["role"], "content": strings.Join(text, "\n") + "\n(The images were left out: the model does not take images.)"})
	}
	return out
}

// agentTools is the tool list in OpenAI function format: the MCP tools plus
// the runner's own.
func (h *AgentHandler) agentTools() []map[string]any {
	defs := append(h.MCP.toolDefinitions(), agentBuiltinToolDefinitions()...)
	tools := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        d.Name,
				"description": d.Description,
				"parameters":  d.InputSchema,
			},
		})
	}
	return tools
}

func agentBuiltinToolDefinitions() []mcpTool {
	return []mcpTool{
		{
			Name:        "web_search",
			Description: "Search the web for current information. Returns the most relevant results with title, URL, date when known and a short summary, plus the source URLs.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      map[string]any{"type": "string", "description": "What to search for. Specific queries work best, e.g. a game name plus \"announcement\" or a publisher name plus a month."},
					"maxResults": map[string]any{"type": "integer", "description": "Number of results (1-10, default 5)"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "fetch_url",
			Description: "Read a web page: its title, description, main image (og:image), publication date when the page states one, and its text.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":      map[string]any{"type": "string", "description": "http(s) URL of the page"},
					"maxChars": map[string]any{"type": "integer", "description": "Maximum characters of page text (default 12000, max 40000)"},
				},
				"required": []string{"url"},
			},
		},
	}
}

// execute runs one agent run to the end and records the outcome.
func (h *AgentHandler) execute(ctx context.Context, a models.Agent, runID primitive.ObjectID) {
	runLog := &agentRunLog{db: h.DB, runID: runID}
	fail := func(msg string) {
		h.finishRun(runID, a.ID, models.AgentRunError, "", msg)
	}

	loadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	var user models.User
	err := h.DB.Users().FindOne(loadCtx, bson.M{"_id": a.UserID}).Decode(&user)
	cancel()
	if err != nil {
		fail("The agent's user no longer exists")
		return
	}
	settings := models.AgentSettings{}
	if user.Agent != nil {
		settings = *user.Agent
	}
	if settings.OpenRouterKey == "" {
		fail("No OpenRouter key: set one on the Profile page under Agents")
		return
	}
	model := a.Model
	if model == "" {
		model = settings.Model
	}
	if model == "" {
		fail("No model: pick one on the Profile page under Agents, or on the agent")
		return
	}
	modelCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	h.DB.AgentRuns().UpdateByID(modelCtx, runID, bson.M{"$set": bson.M{"model": model}})
	cancel()

	maxSteps := a.MaxSteps
	if maxSteps <= 0 {
		maxSteps = defaultAgentMaxSteps
	}
	if maxSteps > maxAgentMaxSteps {
		maxSteps = maxAgentMaxSteps
	}

	rc := agentToolContext(ctx, user)
	conv := &agentConversation{
		key:      settings.OpenRouterKey,
		model:    model,
		messages: chatMessages(agentSystemPrompt(a, settings.GlobalPrompt, time.Now()), a.Prompt),
		tools:    h.agentTools(),
		maxSteps: maxSteps,
		origin:   agentOrigin(a.Name),
		callTool: func(name string, args map[string]any) (any, bool) {
			return h.callAgentTool(ctx, rc, settings.OpenRouterKey, model, name, args)
		},
		onStep:  runLog.add,
		onUsage: runLog.usage,
	}
	status, summary, errMsg := conv.run(ctx)
	h.finishRun(runID, a.ID, status, summary, errMsg)
}

// agentConversation is the model/tool loop of one run, apart from where its
// log goes and how tools run.
type agentConversation struct {
	key, model string
	messages   []map[string]any
	tools      []map[string]any
	maxSteps   int
	origin     string
	callTool   func(name string, args map[string]any) (any, bool)
	onStep     func(models.AgentRunStep)
	onUsage    func(agentUsage)
	noImages   bool
}

// run talks to the model until it answers without a tool call, the step
// limit is reached, or ctx ends. It returns the run status, the model's
// closing summary and an error message.
func (cv *agentConversation) run(ctx context.Context) (status, summary, errMsg string) {
	for step := 0; step < cv.maxSteps; step++ {
		if ctx.Err() != nil {
			break
		}
		resp, err := agentChat(ctx, cv.key, cv.model, cv.messages, cv.tools)
		if errors.Is(err, errImagesUnsupported) && hasImageParts(cv.messages) {
			cv.noImages = true
			cv.messages = withoutImageParts(cv.messages)
			resp, err = agentChat(ctx, cv.key, cv.model, cv.messages, cv.tools)
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return models.AgentRunError, "", err.Error()
		}
		if resp.Usage != nil {
			cv.onUsage(*resp.Usage)
		}
		msg := resp.Choices[0].Message
		text := messageText(msg)
		calls := parseToolCalls(msg)
		if len(calls) == 0 {
			if text == "" {
				text = "(The model ended the run without a summary.)"
			}
			return models.AgentRunOK, text, ""
		}
		if text != "" {
			cv.onStep(models.AgentRunStep{Kind: "message", Text: text})
		}
		cv.messages = append(cv.messages, assistantMessage(msg))

		var images []mcpImage
		var imageTools []string
		for _, call := range calls {
			args := map[string]any{}
			var result any
			isErr := false
			if strings.TrimSpace(call.Args) != "" {
				if err := json.Unmarshal([]byte(call.Args), &args); err != nil {
					result, isErr = map[string]string{"error": "the arguments are not valid JSON: " + err.Error()}, true
				}
			}
			if result == nil {
				if agentCreatingTools[call.Name] && strArg(args, "origin") == "" {
					args["origin"] = cv.origin
				}
				result, isErr = cv.callTool(call.Name, args)
			}
			if rich, ok := result.(*mcpRichResult); ok {
				result = rich.Result
				if !cv.noImages {
					for _, img := range rich.Images {
						if len(images) < maxAgentImages {
							images = append(images, img)
							imageTools = append(imageTools, call.Name)
						}
					}
				}
			}
			raw, _ := json.Marshal(result)
			cv.messages = append(cv.messages, map[string]any{
				"role": "tool", "tool_call_id": call.ID, "name": call.Name,
				"content": truncateText(string(raw), agentToolResultLimit),
			})
			cv.onStep(models.AgentRunStep{Kind: "tool", Tool: call.Name, Args: call.Args, Result: string(raw), IsError: isErr})
		}
		// Tool messages carry text only; images follow as a user turn.
		if len(images) > 0 {
			parts := []map[string]any{{"type": "text", "text": fmt.Sprintf("Images returned by %s:", strings.Join(uniqueStrings(imageTools), ", "))}}
			for _, img := range images {
				parts = append(parts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": "data:" + img.MimeType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)},
				})
			}
			cv.messages = append(cv.messages, map[string]any{"role": "user", "content": parts})
		}
	}

	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return models.AgentRunError, "", fmt.Sprintf("Stopped after %d minutes", int(agentRunTimeout.Minutes()))
		}
		return models.AgentRunStopped, "", "Stopped by the user"
	}
	return models.AgentRunLimit, "", fmt.Sprintf("Stopped at the limit of %d model turns", cv.maxSteps)
}

// callAgentTool runs one tool: the runner's own, or an MCP tool.
func (h *AgentHandler) callAgentTool(ctx context.Context, rc *gin.Context, key, model, name string, args map[string]any) (result any, isErr bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[agents] tool %s panicked: %v", name, r)
			result, isErr = map[string]string{"error": fmt.Sprintf("the tool failed: %v", r)}, true
		}
	}()
	switch name {
	case "web_search":
		return agentWebSearch(ctx, key, model, args)
	case "fetch_url":
		return agentFetchURL(ctx, args)
	}
	return h.MCP.callTool(rc, name, args)
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// truncateText cuts s to at most n bytes on a rune boundary, saying so.
func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf(" [... %d more characters cut]", len(s)-cut)
}

const webSearchSystemPrompt = `You are a web search assistant. Using only the web search results you were given, list the results most relevant to the query.
For each result give: the title, the URL, the publication date when known, and two sentences on what it says.
Never invent a result or a URL. If nothing relevant was found, say so.`

// agentWebSearch runs a search through OpenRouter's web plugin with the
// agent's own model and key, and returns the answer with its sources.
func agentWebSearch(ctx context.Context, key, model string, args map[string]any) (any, bool) {
	query := strings.TrimSpace(strArg(args, "query"))
	if query == "" {
		return map[string]string{"error": "query is required"}, true
	}
	n := intArg(args, "maxResults", 5)
	if n < 1 {
		n = 1
	}
	if n > 10 {
		n = 10
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":    model,
		"messages": chatMessages(webSearchSystemPrompt, "Query: "+query),
		"plugins":  []map[string]any{{"id": "web", "max_results": n}},
	})
	sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(sctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "SocialPod Agents")
	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		return map[string]string{"error": "web search failed: " + err.Error()}, true
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if resp.StatusCode != http.StatusOK {
		return map[string]string{"error": fmt.Sprintf("web search failed (%d): %s", resp.StatusCode, truncateText(string(body), 500))}, true
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content     string `json:"content"`
				Annotations []struct {
					Type        string `json:"type"`
					URLCitation struct {
						URL     string `json:"url"`
						Title   string `json:"title"`
						Content string `json:"content"`
					} `json:"url_citation"`
				} `json:"annotations"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil || len(out.Choices) == 0 {
		return map[string]string{"error": "web search returned no answer"}, true
	}
	msg := out.Choices[0].Message
	type source struct {
		URL     string `json:"url"`
		Title   string `json:"title,omitempty"`
		Snippet string `json:"snippet,omitempty"`
	}
	var sources []source
	seen := map[string]bool{}
	for _, a := range msg.Annotations {
		if a.Type != "url_citation" || a.URLCitation.URL == "" || seen[a.URLCitation.URL] {
			continue
		}
		seen[a.URLCitation.URL] = true
		sources = append(sources, source{URL: a.URLCitation.URL, Title: a.URLCitation.Title, Snippet: truncateText(a.URLCitation.Content, 500)})
	}
	return map[string]any{"query": query, "results": strings.TrimSpace(msg.Content), "sources": sources}, false
}

var (
	htmlDropRe    = regexp.MustCompile(`(?is)<(script|style|noscript|svg|template|iframe)\b.*?</(script|style|noscript|svg|template|iframe)>`)
	htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlBlockRe   = regexp.MustCompile(`(?i)</?(p|div|br|h[1-6]|li|tr|article|section|header|footer|blockquote)\b[^>]*>`)
	spacesRe      = regexp.MustCompile(`[ \t\r\f\v]+`)
	blankLinesRe  = regexp.MustCompile(`\n\s*\n+`)
	htmlTitleRe   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// htmlToText reduces a page to readable text with paragraph breaks.
func htmlToText(page string) string {
	s := htmlCommentRe.ReplaceAllString(page, " ")
	s = htmlDropRe.ReplaceAllString(s, " ")
	s = htmlBlockRe.ReplaceAllString(s, "\n")
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = spacesRe.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	s = strings.Join(lines, "\n")
	s = blankLinesRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// agentFetchURL reads a web page through the SSRF-safe client.
func agentFetchURL(ctx context.Context, args map[string]any) (any, bool) {
	raw := strings.TrimSpace(strArg(args, "url"))
	u, err := url.Parse(raw)
	if raw == "" || err != nil {
		return map[string]string{"error": "a valid url is required"}, true
	}
	if err := validateURL(u); err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	maxChars := intArg(args, "maxChars", 12000)
	if maxChars < 500 {
		maxChars = 500
	}
	if maxChars > agentToolResultLimit {
		maxChars = agentToolResultLimit
	}
	fctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(fctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")
	resp, err := safeURLClient.Do(req)
	if err != nil {
		return map[string]string{"error": "fetch failed: " + err.Error()}, true
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if resp.StatusCode != http.StatusOK {
		return map[string]any{"error": fmt.Sprintf("the page answered %d", resp.StatusCode), "url": resp.Request.URL.String()}, true
	}
	ct := resp.Header.Get("Content-Type")
	out := map[string]any{"url": resp.Request.URL.String(), "contentType": ct}
	page := string(body)
	if strings.Contains(ct, "html") || strings.Contains(strings.ToLower(page[:min(len(page), 512)]), "<html") {
		title := extractMetaContent(page, "og:title")
		if title == "" {
			if m := htmlTitleRe.FindStringSubmatch(page); len(m) > 1 {
				title = strings.TrimSpace(html.UnescapeString(m[1]))
			}
		}
		out["title"] = title
		if v := extractMetaContent(page, "og:description", "description"); v != "" {
			out["description"] = v
		}
		if v := extractMetaContent(page, "og:image"); v != "" {
			if ref, err := u.Parse(v); err == nil {
				v = ref.String()
			}
			out["image"] = v
		}
		if v := extractMetaContent(page, "article:published_time", "date", "pubdate"); v != "" {
			out["published"] = v
		}
		page = htmlToText(page)
	} else if !strings.HasPrefix(ct, "text/") && !strings.Contains(ct, "json") && !strings.Contains(ct, "xml") {
		return map[string]any{"url": out["url"], "contentType": ct, "error": "not a text page; use upload_from_url for images"}, true
	}
	out["text"] = truncateText(page, maxChars)
	return out, false
}
