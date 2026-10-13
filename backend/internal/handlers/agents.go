package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// agentView is an agent as the API returns it, with the origin label its
// drafts carry.
type agentView struct {
	models.Agent
	Origin string
}

func viewAgent(a models.Agent) agentView {
	return agentView{Agent: a, Origin: agentOrigin(a.Name)}
}

// MarshalJSON adds origin to the agent's own fields.
func (v agentView) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(v.Agent)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["origin"] = v.Origin
	return json.Marshal(m)
}

// agentUser resolves the signed-in user. Agents belong to a person and use
// their OpenRouter key, so a team token cannot manage them.
func agentUser(c *gin.Context) (primitive.ObjectID, bool) {
	if isTeam, ok := c.Get("isTeamToken"); ok && isTeam.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Agents need a user account; a team token cannot manage them"})
		return primitive.NilObjectID, false
	}
	userID, _ := c.Get("userId")
	s, _ := userID.(string)
	uid, err := primitive.ObjectIDFromHex(s)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unknown user"})
		return primitive.NilObjectID, false
	}
	return uid, true
}

func (h *AgentHandler) loadUser(ctx context.Context, uid primitive.ObjectID) (models.User, error) {
	var u models.User
	err := h.DB.Users().FindOne(ctx, bson.M{"_id": uid}).Decode(&u)
	return u, err
}

func agentSettingsResponse(u models.User) gin.H {
	s := models.AgentSettings{}
	if u.Agent != nil {
		s = *u.Agent
	}
	hint := ""
	if k := s.OpenRouterKey; len(k) > 4 {
		hint = "…" + k[len(k)-4:]
	}
	return gin.H{
		"hasKey":       s.OpenRouterKey != "",
		"keyHint":      hint,
		"model":        s.Model,
		"globalPrompt": s.GlobalPrompt,
	}
}

// GetSettings returns the user's agent settings, never the key itself.
// GET /api/agents/settings
func (h *AgentHandler) GetSettings(c *gin.Context) {
	uid, ok := agentUser(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u, err := h.loadUser(ctx, uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.JSON(http.StatusOK, agentSettingsResponse(u))
}

// verifyOpenRouterKey asks OpenRouter whether a key is valid. Only an
// explicit rejection counts; an unreachable OpenRouter does not block saving.
var verifyOpenRouterKey = func(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/key", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("OpenRouter rejected this key")
	}
	return nil
}

// UpdateSettings changes the fields it is given. An empty openRouterKey
// removes the stored key.
// PUT /api/agents/settings {openRouterKey?, model?, globalPrompt?}
func (h *AgentHandler) UpdateSettings(c *gin.Context) {
	uid, ok := agentUser(c)
	if !ok {
		return
	}
	var input struct {
		OpenRouterKey *string `json:"openRouterKey"`
		Model         *string `json:"model"`
		GlobalPrompt  *string `json:"globalPrompt"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	set := bson.M{"updatedAt": time.Now()}
	unset := bson.M{}
	if input.OpenRouterKey != nil {
		key := strings.TrimSpace(*input.OpenRouterKey)
		if key == "" {
			unset["agent.openRouterKey"] = ""
		} else {
			if err := verifyOpenRouterKey(ctx, key); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			set["agent.openRouterKey"] = key
		}
	}
	if input.Model != nil {
		set["agent.model"] = strings.TrimSpace(*input.Model)
	}
	if input.GlobalPrompt != nil {
		if len(*input.GlobalPrompt) > 20000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "The global prompt is limited to 20,000 characters"})
			return
		}
		set["agent.globalPrompt"] = strings.TrimSpace(*input.GlobalPrompt)
	}
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	// A user saved before agents existed has no agent sub-document; $set on
	// "agent.x" creates it, but not when agent is null.
	h.DB.Users().UpdateOne(ctx, bson.M{"_id": uid, "agent": nil}, bson.M{"$set": bson.M{"agent": bson.M{}}})
	if _, err := h.DB.Users().UpdateOne(ctx, bson.M{"_id": uid}, update); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save"})
		return
	}
	u, err := h.loadUser(ctx, uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.JSON(http.StatusOK, agentSettingsResponse(u))
}

// agentModel is an OpenRouter model that can call tools. Prices are in US
// dollars per million tokens.
type agentModel struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	ContextLength   int     `json:"contextLength"`
	PromptPrice     float64 `json:"promptPrice"`
	CompletionPrice float64 `json:"completionPrice"`
	Images          bool    `json:"images"`
}

var agentModelCache struct {
	sync.Mutex
	models  []agentModel
	fetched time.Time
}

// ListModels lists the OpenRouter models that support tool calling, cached
// for an hour.
// GET /api/agents/models
func (h *AgentHandler) ListModels(c *gin.Context) {
	agentModelCache.Lock()
	defer agentModelCache.Unlock()
	if agentModelCache.models != nil && time.Since(agentModelCache.fetched) < time.Hour {
		c.JSON(http.StatusOK, agentModelCache.models)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/models?supported_parameters=tools", nil)
	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Could not reach OpenRouter"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": "OpenRouter did not list its models"})
		return
	}
	list, err := parseAgentModels(body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Unexpected model list from OpenRouter"})
		return
	}
	agentModelCache.models = list
	agentModelCache.fetched = time.Now()
	c.JSON(http.StatusOK, list)
}

func parseAgentModels(body []byte) ([]agentModel, error) {
	var raw struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
			Architecture struct {
				InputModalities []string `json:"input_modalities"`
			} `json:"architecture"`
			SupportedParameters []string `json:"supported_parameters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	perMillion := func(s string) float64 {
		var f float64
		if err := json.Unmarshal([]byte(s), &f); err != nil {
			return 0
		}
		return f * 1e6
	}
	list := []agentModel{}
	for _, m := range raw.Data {
		tools := false
		for _, p := range m.SupportedParameters {
			if p == "tools" {
				tools = true
			}
		}
		if !tools {
			continue
		}
		images := false
		for _, mod := range m.Architecture.InputModalities {
			if mod == "image" {
				images = true
			}
		}
		list = append(list, agentModel{
			ID: m.ID, Name: m.Name, ContextLength: m.ContextLength,
			PromptPrice: perMillion(m.Pricing.Prompt), CompletionPrice: perMillion(m.Pricing.Completion),
			Images: images,
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

type agentInput struct {
	Name     string               `json:"name"`
	Prompt   string               `json:"prompt"`
	Enabled  bool                 `json:"enabled"`
	Schedule models.AgentSchedule `json:"schedule"`
	Model    string               `json:"model"`
	MaxSteps int                  `json:"maxSteps"`
}

func (in *agentInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Prompt = strings.TrimSpace(in.Prompt)
	in.Model = strings.TrimSpace(in.Model)
	if in.Name == "" {
		return errors.New("Give the agent a name")
	}
	if len(in.Name) > 80 {
		return errors.New("The name is limited to 80 characters")
	}
	if in.Prompt == "" {
		return errors.New("Give the agent a prompt")
	}
	if len(in.Prompt) > 20000 {
		return errors.New("The prompt is limited to 20,000 characters")
	}
	if in.MaxSteps < 0 || in.MaxSteps > maxAgentMaxSteps {
		return errors.New("Max steps must be between 1 and 100")
	}
	sched, err := normalizeAgentSchedule(in.Schedule)
	if err != nil {
		msg := err.Error()
		return errors.New(strings.ToUpper(msg[:1]) + msg[1:])
	}
	in.Schedule = sched
	return nil
}

// List returns the user's agents.
// GET /api/agents
func (h *AgentHandler) List(c *gin.Context) {
	uid, ok := agentUser(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cur, err := h.DB.Agents().Find(ctx, bson.M{"userId": uid}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load agents"})
		return
	}
	var agents []models.Agent
	if err := cur.All(ctx, &agents); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load agents"})
		return
	}
	out := make([]agentView, 0, len(agents))
	for _, a := range agents {
		out = append(out, viewAgent(a))
	}
	c.JSON(http.StatusOK, out)
}

// Create adds an agent.
// POST /api/agents
func (h *AgentHandler) Create(c *gin.Context) {
	uid, ok := agentUser(c)
	if !ok {
		return
	}
	var in agentInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	if err := in.validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	u, err := h.loadUser(ctx, uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	now := time.Now().UTC()
	a := models.Agent{
		ID:        primitive.NewObjectID(),
		UserID:    uid,
		TeamID:    u.TeamID,
		Name:      in.Name,
		Prompt:    in.Prompt,
		Enabled:   in.Enabled,
		Schedule:  in.Schedule,
		Model:     in.Model,
		MaxSteps:  in.MaxSteps,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if a.Enabled {
		a.NextRunAt = nextAgentRun(a.Schedule, now)
	}
	if _, err := h.DB.Agents().InsertOne(ctx, a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save the agent"})
		return
	}
	c.JSON(http.StatusCreated, viewAgent(a))
}

func (h *AgentHandler) ownedFilter(c *gin.Context) (bson.M, bool) {
	uid, ok := agentUser(c)
	if !ok {
		return nil, false
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return nil, false
	}
	return bson.M{"_id": id, "userId": uid}, true
}

// Update replaces an agent's settings and recomputes its next run.
// PUT /api/agents/:id
func (h *AgentHandler) Update(c *gin.Context) {
	filter, ok := h.ownedFilter(c)
	if !ok {
		return
	}
	var in agentInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	if err := in.validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now().UTC()
	set := bson.M{
		"name": in.Name, "prompt": in.Prompt, "enabled": in.Enabled, "schedule": in.Schedule,
		"model": in.Model, "maxSteps": in.MaxSteps, "updatedAt": now,
	}
	update := bson.M{"$set": set}
	var next *time.Time
	if in.Enabled {
		next = nextAgentRun(in.Schedule, now)
	}
	if next != nil {
		set["nextRunAt"] = *next
	} else {
		update["$unset"] = bson.M{"nextRunAt": ""}
	}
	var a models.Agent
	err := h.DB.Agents().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&a)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}
	c.JSON(http.StatusOK, viewAgent(a))
}

// Delete removes an agent and its run history, stopping a run in progress.
// DELETE /api/agents/:id
func (h *AgentHandler) Delete(c *gin.Context) {
	filter, ok := h.ownedFilter(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := h.DB.Agents().DeleteOne(ctx, filter)
	if err != nil || res.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}
	id := filter["_id"].(primitive.ObjectID)
	h.cancelRun(id)
	h.DB.AgentRuns().DeleteMany(ctx, bson.M{"agentId": id})
	c.JSON(http.StatusOK, gin.H{"message": "Agent deleted"})
}

// Run starts an agent now, whatever its schedule.
// POST /api/agents/:id/run
func (h *AgentHandler) Run(c *gin.Context) {
	filter, ok := h.ownedFilter(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	u, err := h.loadUser(ctx, filter["userId"].(primitive.ObjectID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if u.Agent == nil || u.Agent.OpenRouterKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Set your OpenRouter key on the Profile page under Agents first"})
		return
	}
	var existing models.Agent
	if err := h.DB.Agents().FindOne(ctx, filter).Decode(&existing); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}
	if existing.Model == "" && u.Agent.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pick a model on the Profile page under Agents first"})
		return
	}
	a, err := h.claimManual(ctx, filter)
	if errors.Is(err, mongo.ErrNoDocuments) {
		c.JSON(http.StatusConflict, gin.H{"error": "The agent is already running"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start the agent"})
		return
	}
	runID, err := h.start(a, "manual")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start the agent"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"runId": runID.Hex()})
}

// Stop cancels a run in progress.
// POST /api/agents/:id/stop
func (h *AgentHandler) Stop(c *gin.Context) {
	filter, ok := h.ownedFilter(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var a models.Agent
	if err := h.DB.Agents().FindOne(ctx, filter).Decode(&a); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}
	if !h.cancelRun(a.ID) {
		// Not running in this process: a stale flag; clear it.
		if a.Running {
			h.DB.AgentRuns().UpdateMany(ctx, bson.M{"agentId": a.ID, "status": models.AgentRunRunning}, bson.M{"$set": bson.M{
				"status": models.AgentRunStopped, "error": "Stopped by the user", "finishedAt": time.Now().UTC(),
			}})
			h.finishAgent(a.ID, models.AgentRunStopped, "Stopped by the user")
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "Stopping"})
}

// ListRuns returns an agent's recent runs without their step logs.
// GET /api/agents/:id/runs
func (h *AgentHandler) ListRuns(c *gin.Context) {
	filter, ok := h.ownedFilter(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	opts := options.Find().SetSort(bson.D{{Key: "startedAt", Value: -1}}).SetLimit(50).SetProjection(bson.M{"steps": 0})
	cur, err := h.DB.AgentRuns().Find(ctx, bson.M{"agentId": filter["_id"], "userId": filter["userId"]}, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load runs"})
		return
	}
	runs := []models.AgentRun{}
	if err := cur.All(ctx, &runs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load runs"})
		return
	}
	c.JSON(http.StatusOK, runs)
}

// GetRun returns one run with its step log.
// GET /api/agents/:id/runs/:runId
func (h *AgentHandler) GetRun(c *gin.Context) {
	filter, ok := h.ownedFilter(c)
	if !ok {
		return
	}
	runID, err := primitive.ObjectIDFromHex(c.Param("runId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid run ID"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var run models.AgentRun
	if err := h.DB.AgentRuns().FindOne(ctx, bson.M{"_id": runID, "agentId": filter["_id"], "userId": filter["userId"]}).Decode(&run); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Run not found"})
		return
	}
	if run.Steps == nil {
		run.Steps = []models.AgentRunStep{}
	}
	c.JSON(http.StatusOK, run)
}
