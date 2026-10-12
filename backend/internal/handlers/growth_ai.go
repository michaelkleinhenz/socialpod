package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// DefaultInstagramRewritePrompt is the system prompt "Rewrite for Instagram"
// uses unless the team set its own. It carries the growth plan's caption
// rules and tone.
const DefaultInstagramRewritePrompt = `You rewrite social media posts into Instagram captions for a board game podcast.
Tone: knowledgeable, dry humour, a clear opinion. Never bland filler like "we are excited". Say what you actually think.
Rules:
1. The first line is a hook that stops the scroll (under 80 characters).
2. State one clear opinion.
3. Add a share prompt, e.g. "Send this to the person in your group who always explains the rules."
4. End the text with a question to the community.
5. Keep every @mention of the original.
6. Finish with 3 to 5 specific hashtags on their own line, never more.
7. Do not copy the original wording one to one; this is not a cross-post.
Reply with ONLY the caption, no quotes, no commentary.`

// openRouterChat runs one chat completion with the configured model.
func openRouterChat(ctx context.Context, settings models.AppSettings, system, user string) (string, error) {
	model := settings.OpenRouterModel
	if model == "" {
		model = "openai/gpt-4o-mini"
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":    model,
		"messages": chatMessages(system, user),
	})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+settings.OpenRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to reach OpenRouter: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenRouter returned %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) == 0 {
		return "", fmt.Errorf("invalid response from OpenRouter")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// RewriteInstagram turns a post text into an Instagram caption in the team's
// tone.
// POST /api/growth/rewrite-instagram {"text": "..."}
func (h *GrowthHandler) RewriteInstagram(c *gin.Context) {
	var input struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Text) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a text to rewrite first"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil || settings.OpenRouterAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OpenRouter is not configured"})
		return
	}

	system := DefaultInstagramRewritePrompt
	if teamID := teamIDFrom(c); teamID != nil {
		var team models.Team
		if h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team) == nil && team.Growth != nil && strings.TrimSpace(team.Growth.InstagramRewritePrompt) != "" {
			system = team.Growth.InstagramRewritePrompt
		}
	}
	system += fmt.Sprintf(" The caption must fit within %d characters.", instagramCharLimit)
	if settings.AILanguage != "" {
		system += fmt.Sprintf(" Write in %s.", settings.AILanguage)
	}

	text, err := openRouterChat(ctx, settings, system, input.Text)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"text": text})
}
