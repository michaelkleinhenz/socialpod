package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	htmlpkg "golang.org/x/net/html"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BGGHandler struct {
	DB        *database.MongoDB
	UploadDir string
}

// BGG XML API structures

type bggItems struct {
	XMLName xml.Name  `xml:"items"`
	Items   []bggItem `xml:"item"`
}

type bggItem struct {
	Names      []bggName     `xml:"name"`
	Thumbnail  string        `xml:"thumbnail"`
	Image      string        `xml:"image"`
	YearPub    bggVal        `xml:"yearpublished"`
	MinPlayers bggVal        `xml:"minplayers"`
	MaxPlayers bggVal        `xml:"maxplayers"`
	MinTime    bggVal        `xml:"minplaytime"`
	MaxTime    bggVal        `xml:"maxplaytime"`
	MinAge     bggVal        `xml:"minage"`
	Desc       string        `xml:"description"`
	Links      []bggLink     `xml:"link"`
	Stats      bggStatistics `xml:"statistics"`
}

type bggName struct {
	Type  string `xml:"type,attr"`
	Value string `xml:"value,attr"`
}

type bggVal struct {
	Value string `xml:"value,attr"`
}

type bggLink struct {
	Type  string `xml:"type,attr"`
	Value string `xml:"value,attr"`
}

type bggStatistics struct {
	Ratings bggRatings `xml:"ratings"`
}

type bggRatings struct {
	Average bggVal `xml:"average"`
	Weight  bggVal `xml:"averageweight"`
}

type BGGGameResponse struct {
	GameID           string   `json:"gameId"`
	Title            string   `json:"title"`
	YearPublished    string   `json:"yearPublished,omitempty"`
	MinPlayers       string   `json:"minPlayers"`
	MaxPlayers       string   `json:"maxPlayers"`
	MinPlaytime      string   `json:"minPlaytime"`
	MaxPlaytime      string   `json:"maxPlaytime"`
	MinAge           string   `json:"minAge"`
	Designers        []string `json:"designers"`
	Artists          []string `json:"artists"`
	Publishers       []string `json:"publishers"`
	Rating           string   `json:"rating,omitempty"`
	Weight           string   `json:"weight,omitempty"`
	ImageBase64      string   `json:"imageBase64"`
	ImageFilename    string   `json:"imageFilename"`
	SuggestedContent string   `json:"suggestedContent"`
}

var (
	bggGameIDRe = regexp.MustCompile(`/boardgame[^/]*/(\d+)`)
	htmlTagRe   = regexp.MustCompile(`<[^>]+>`)
	multiNLRe   = regexp.MustCompile(`\n{3,}`)
)

// FetchGame handles GET /api/bgg/fetch?url=...
func (h *BGGHandler) FetchGame(c *gin.Context) {
	rawURL := c.Query("url")
	if rawURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url parameter is required"})
		return
	}

	m := bggGameIDRe.FindStringSubmatch(rawURL)
	if m == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not extract a game ID from the BGG URL"})
		return
	}
	gameID := m[1]

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var settings models.AppSettings
	h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings)

	method := settings.BGGFetchMethod
	if method == "" {
		method = "scrape"
	}

	var item bggItem
	var err error
	switch method {
	case "api":
		item, err = h.fetchBGGItem(ctx, gameID, settings.BGGAPIToken)
	default:
		item, err = h.scrapeBGGGame(ctx, rawURL)
		if err != nil {
			// BGG frequently blocks server-side scraping (HTTP 403 via Cloudflare).
			// Fall back to the public BGG XML API, which works without authentication.
			var apiErr error
			item, apiErr = h.fetchBGGItem(ctx, gameID, settings.BGGAPIToken)
			if apiErr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
				return
			}
			err = nil
		}
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	title := ""
	for _, n := range item.Names {
		if n.Type == "primary" {
			title = n.Value
			break
		}
	}
	if title == "" && len(item.Names) > 0 {
		title = item.Names[0].Value
	}

	var designers, artists, publishers []string
	for _, link := range item.Links {
		switch link.Type {
		case "boardgamedesigner":
			designers = append(designers, link.Value)
		case "boardgameartist":
			artists = append(artists, link.Value)
		case "boardgamepublisher":
			publishers = append(publishers, link.Value)
		}
	}

	description := cleanBGGText(item.Desc)

	// Generate AI one-sentence summary (best-effort; omitted if not configured)
	aiSummary := ""
	if settings.OpenRouterAPIKey != "" {
		if s, err := h.generateGameSummary(ctx, description, title, settings); err == nil {
			aiSummary = s
		}
	}

	// Download and process cover image (best-effort)
	imageBase64 := ""
	imageFilename := ""
	imgURL := strings.TrimSpace(item.Image)
	if imgURL == "" {
		imgURL = strings.TrimSpace(item.Thumbnail)
	}
	if imgURL != "" {
		if strings.HasPrefix(imgURL, "//") {
			imgURL = "https:" + imgURL
		}
		if data, err := h.downloadAndProcess(ctx, c, imgURL); err == nil {
			imageBase64 = base64.StdEncoding.EncodeToString(data)
			imageFilename = fmt.Sprintf("bgg-%s.jpg", gameID)
		}
	}

	resp := BGGGameResponse{
		GameID:           gameID,
		Title:            title,
		YearPublished:    item.YearPub.Value,
		MinPlayers:       item.MinPlayers.Value,
		MaxPlayers:       item.MaxPlayers.Value,
		MinPlaytime:      item.MinTime.Value,
		MaxPlaytime:      item.MaxTime.Value,
		MinAge:           item.MinAge.Value,
		Designers:        nilSlice(designers),
		Artists:          nilSlice(artists),
		Publishers:       nilSlice(publishers),
		Rating:           trimFloat(item.Stats.Ratings.Average.Value),
		Weight:           trimFloat(item.Stats.Ratings.Weight.Value),
		ImageBase64:      imageBase64,
		ImageFilename:    imageFilename,
		SuggestedContent: buildPostContent(title, designers, artists, publishers, item, aiSummary),
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BGGHandler) fetchBGGItem(ctx context.Context, gameID string, token string) (bggItem, error) {
	apiURL := fmt.Sprintf("https://boardgamegeek.com/xmlapi2/thing?id=%s&stats=1", gameID)

	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return bggItem{}, fmt.Errorf("BGG API timeout")
			case <-time.After(2 * time.Second):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return bggItem{}, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SocialPod/1.0)")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Referer", "https://boardgamegeek.com/")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return bggItem{}, fmt.Errorf("failed to reach BGG API: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusAccepted {
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return bggItem{}, fmt.Errorf("BGG API authentication failed — add your BGG API token in Admin › Settings")
		}
		if resp.StatusCode != http.StatusOK {
			return bggItem{}, fmt.Errorf("BGG API returned %d", resp.StatusCode)
		}

		var items bggItems
		if err := xml.Unmarshal(body, &items); err != nil {
			return bggItem{}, fmt.Errorf("failed to parse BGG XML: %w", err)
		}
		if len(items.Items) == 0 {
			return bggItem{}, fmt.Errorf("game not found on BGG")
		}
		return items.Items[0], nil
	}
	return bggItem{}, fmt.Errorf("BGG API not ready, please try again in a moment")
}

// scrapeBGGGame fetches a BGG game page and extracts game data via HTML scraping.
// This is the default method used while awaiting BGG API approval.
func (h *BGGHandler) scrapeBGGGame(ctx context.Context, pageURL string) (bggItem, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return bggItem{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://boardgamegeek.com/")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return bggItem{}, fmt.Errorf("failed to fetch BGG page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return bggItem{}, fmt.Errorf("BGG rate limit exceeded, please try again later")
	}
	if resp.StatusCode == http.StatusForbidden {
		return bggItem{}, fmt.Errorf("BGG blocked the scraper request (HTTP 403)")
	}
	if resp.StatusCode != http.StatusOK {
		return bggItem{}, fmt.Errorf("BGG page returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return bggItem{}, err
	}
	return parseBGGPage(body)
}

func parseBGGPage(body []byte) (bggItem, error) {
	jsonLDs, metas := extractHTMLData(body)

	item := bggItem{}

	for _, raw := range jsonLDs {
		data := map[string]interface{}{}
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			// Try array of JSON-LD objects
			var arr []map[string]interface{}
			if err2 := json.Unmarshal([]byte(raw), &arr); err2 == nil {
				for _, d := range arr {
					if t, _ := d["@type"].(string); t == "BoardGame" || t == "Game" {
						data = d
						break
					}
				}
			}
		}
		if len(data) == 0 {
			continue
		}
		t, _ := data["@type"].(string)
		if t != "BoardGame" && t != "Game" {
			continue
		}

		if name, ok := data["name"].(string); ok && name != "" {
			item.Names = []bggName{{Type: "primary", Value: name}}
		}
		if desc, ok := data["description"].(string); ok {
			item.Desc = desc
		}

		switch v := data["image"].(type) {
		case string:
			item.Image = v
		case []interface{}:
			if len(v) > 0 {
				if s, ok := v[0].(string); ok {
					item.Image = s
				}
			}
		}

		if players, ok := data["numberOfPlayers"].(map[string]interface{}); ok {
			if min := bggNumStr(players["minValue"]); min != "" && min != "0" {
				item.MinPlayers = bggVal{Value: min}
			}
			if max := bggNumStr(players["maxValue"]); max != "" && max != "0" {
				item.MaxPlayers = bggVal{Value: max}
			}
		}

		if age := bggNumStr(data["suggestedAge"]); age != "" && age != "0" {
			item.MinAge = bggVal{Value: age}
		}

		if rating, ok := data["aggregateRating"].(map[string]interface{}); ok {
			if rv := bggNumStr(rating["ratingValue"]); rv != "" {
				item.Stats.Ratings.Average = bggVal{Value: rv}
			}
		}

		if dateStr, ok := data["datePublished"].(string); ok && len(dateStr) >= 4 {
			item.YearPub = bggVal{Value: dateStr[:4]}
		}

		item.Links = append(item.Links, bggExtractPeople(data["author"], "boardgamedesigner")...)
		item.Links = append(item.Links, bggExtractPeople(data["publisher"], "boardgamepublisher")...)

		break
	}

	if len(item.Names) == 0 {
		if title := metas["og:title"]; title != "" {
			// BGG og:title is often "Game Name | Board Game | BoardGameGeek"
			parts := strings.SplitN(title, " | ", 2)
			item.Names = []bggName{{Type: "primary", Value: strings.TrimSpace(parts[0])}}
		}
	}
	if item.Desc == "" {
		if desc := metas["og:description"]; desc != "" {
			item.Desc = desc
		}
	}
	if item.Image == "" {
		if img := metas["og:image"]; img != "" {
			item.Image = img
		}
	}

	if len(item.Names) == 0 {
		return bggItem{}, fmt.Errorf("could not extract game data from BGG page — try the BGG API method instead")
	}
	return item, nil
}

func extractHTMLData(body []byte) (jsonLDs []string, metas map[string]string) {
	metas = make(map[string]string)
	doc, err := htmlpkg.Parse(bytes.NewReader(body))
	if err != nil {
		return
	}
	var walk func(*htmlpkg.Node)
	walk = func(n *htmlpkg.Node) {
		if n.Type == htmlpkg.ElementNode {
			switch n.Data {
			case "script":
				for _, attr := range n.Attr {
					if attr.Key == "type" && attr.Val == "application/ld+json" {
						if n.FirstChild != nil {
							jsonLDs = append(jsonLDs, n.FirstChild.Data)
						}
					}
				}
			case "meta":
				property, name, content := "", "", ""
				for _, attr := range n.Attr {
					switch attr.Key {
					case "property":
						property = attr.Val
					case "name":
						name = attr.Val
					case "content":
						content = attr.Val
					}
				}
				if property != "" && content != "" {
					metas[property] = content
				}
				if name != "" && content != "" {
					if _, exists := metas["name:"+name]; !exists {
						metas["name:"+name] = content
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return
}

func bggNumStr(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%.4f", val)
	}
	return ""
}

func bggExtractPeople(v interface{}, linkType string) []bggLink {
	var links []bggLink
	switch a := v.(type) {
	case string:
		if a != "" {
			links = append(links, bggLink{Type: linkType, Value: a})
		}
	case map[string]interface{}:
		if name, ok := a["name"].(string); ok && name != "" {
			links = append(links, bggLink{Type: linkType, Value: name})
		}
	case []interface{}:
		for _, item := range a {
			switch p := item.(type) {
			case string:
				if p != "" {
					links = append(links, bggLink{Type: linkType, Value: p})
				}
			case map[string]interface{}:
				if name, ok := p["name"].(string); ok && name != "" {
					links = append(links, bggLink{Type: linkType, Value: name})
				}
			}
		}
	}
	return links
}

func (h *BGGHandler) downloadAndProcess(ctx context.Context, c *gin.Context, imgURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SocialPod/1.0)")
	req.Header.Set("Accept", "image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Referer", "https://boardgamegeek.com/")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image download returned %d", resp.StatusCode)
	}

	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	src, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Crop to square
	src = cropSquare(src)

	// Resize to 1080×1080 for social media
	src = resizeImage(src, 1080, 1080)

	// Overlay team BGG watermark if configured
	if wm := h.loadBGGWatermark(ctx, c); wm != nil {
		bounds := src.Bounds()
		wmScaled := resizeImage(wm, bounds.Dx(), bounds.Dy())
		dst := image.NewRGBA(bounds)
		draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
		draw.Draw(dst, bounds, wmScaled, image.Point{}, draw.Over)
		src = dst
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h *BGGHandler) loadBGGWatermark(ctx context.Context, c *gin.Context) image.Image {
	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		return nil
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		return nil
	}

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil || team.BGGWatermarkID == nil {
		return nil
	}

	var wm models.Watermark
	if err := h.DB.Watermarks().FindOne(ctx, bson.M{"_id": *team.BGGWatermarkID}).Decode(&wm); err != nil {
		return nil
	}

	filename := filepath.Base(wm.URL)
	data, err := os.ReadFile(filepath.Join(h.UploadDir, filename))
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}

func (h *BGGHandler) generateGameSummary(ctx context.Context, description, title string, settings models.AppSettings) (string, error) {
	model := settings.OpenRouterModel
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	systemPrompt := "You are a board game expert. Write exactly one sentence that captures what makes this board game unique and fun to play. Be concise and engaging. Reply with ONLY the single sentence, no quotes, no extra text."
	if settings.AILanguage != "" {
		systemPrompt += fmt.Sprintf(" Write in %s.", settings.AILanguage)
	}

	desc := description
	if len(desc) > 1500 {
		desc = desc[:1500] + "..."
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": fmt.Sprintf("Board game: %s\n\nDescription: %s\n\nWrite one sentence.", title, desc)},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+settings.OpenRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenRouter returned %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) == 0 {
		return "", fmt.Errorf("invalid OpenRouter response")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// GetTeamSettings returns BGG settings for the current team.
func (h *BGGHandler) GetTeamSettings(c *gin.Context) {
	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No team associated with this account"})
		return
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	wmID := ""
	if team.BGGWatermarkID != nil {
		wmID = team.BGGWatermarkID.Hex()
	}
	c.JSON(http.StatusOK, gin.H{"bggWatermarkId": wmID})
}

// UpdateTeamSettings updates BGG settings for the current team.
func (h *BGGHandler) UpdateTeamSettings(c *gin.Context) {
	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No team associated with this account"})
		return
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var input struct {
		BGGWatermarkID *string `json:"bggWatermarkId"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var updateDoc bson.M
	if input.BGGWatermarkID == nil || *input.BGGWatermarkID == "" {
		updateDoc = bson.M{
			"$unset": bson.M{"bggWatermarkId": ""},
			"$set":   bson.M{"updatedAt": time.Now()},
		}
	} else {
		wmID, err := primitive.ObjectIDFromHex(*input.BGGWatermarkID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
			return
		}
		updateDoc = bson.M{"$set": bson.M{"bggWatermarkId": wmID, "updatedAt": time.Now()}}
	}

	result, err := h.DB.Teams().UpdateOne(ctx, bson.M{"_id": teamID}, updateDoc)
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Settings updated"})
}

// --- helpers ---

func buildPostContent(title string, designers, artists, publishers []string, item bggItem, aiSummary string) string {
	var sb strings.Builder

	sb.WriteString(title)
	sb.WriteString("\n\n")

	if aiSummary != "" {
		sb.WriteString(aiSummary)
		sb.WriteString("\n\n")
	}

	// Metadata line
	var meta []string
	if item.MinPlayers.Value != "" && item.MaxPlayers.Value != "" {
		if item.MinPlayers.Value == item.MaxPlayers.Value {
			meta = append(meta, fmt.Sprintf("Players: %s", item.MinPlayers.Value))
		} else {
			meta = append(meta, fmt.Sprintf("Players: %s-%s", item.MinPlayers.Value, item.MaxPlayers.Value))
		}
	}
	if item.MinTime.Value != "" && item.MaxTime.Value != "" {
		if item.MinTime.Value == item.MaxTime.Value {
			meta = append(meta, fmt.Sprintf("Time: %s min", item.MinTime.Value))
		} else {
			meta = append(meta, fmt.Sprintf("Time: %s-%s min", item.MinTime.Value, item.MaxTime.Value))
		}
	}
	if item.MinAge.Value != "" && item.MinAge.Value != "0" {
		meta = append(meta, fmt.Sprintf("Age: %s+", item.MinAge.Value))
	}
	if w := trimFloat(item.Stats.Ratings.Weight.Value); w != "" && w != "0" {
		meta = append(meta, fmt.Sprintf("Weight: %s/5", w))
	}
	if len(meta) > 0 {
		sb.WriteString(strings.Join(meta, " | "))
		sb.WriteString("\n")
	}

	if r := trimFloat(item.Stats.Ratings.Average.Value); r != "" && r != "0" {
		sb.WriteString(fmt.Sprintf("BGG Rating: %s/10\n", r))
	}
	if item.YearPub.Value != "" && item.YearPub.Value != "0" {
		sb.WriteString(fmt.Sprintf("Published: %s\n", item.YearPub.Value))
	}
	if len(publishers) > 0 {
		n := min(3, len(publishers))
		sb.WriteString(fmt.Sprintf("Publisher: %s\n", strings.Join(publishers[:n], ", ")))
	}

	sb.WriteString("\n")

	if len(designers) > 0 {
		sb.WriteString(fmt.Sprintf("Designer: %s\n", strings.Join(designers, ", ")))
	}
	if len(artists) > 0 {
		n := min(3, len(artists))
		sb.WriteString(fmt.Sprintf("Artist: %s\n", strings.Join(artists[:n], ", ")))
	}

	return strings.TrimRight(sb.String(), "\n")
}

func cleanBGGText(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagRe.ReplaceAllString(s, "")
	s = multiNLRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func trimFloat(s string) string {
	if s == "" || s == "0" {
		return ""
	}
	// Keep at most 4 significant characters (e.g. "7.69")
	if len(s) > 4 {
		s = s[:4]
	}
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

func nilSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func cropSquare(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == h {
		return img
	}
	size := w
	ox, oy := 0, 0
	if h < w {
		size = h
		ox = (w - h) / 2
	} else {
		size = w
		oy = (h - w) / 2
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(dst, dst.Bounds(), img, image.Point{b.Min.X + ox, b.Min.Y + oy}, draw.Src)
	return dst
}

func resizeImage(img image.Image, newW, newH int) image.Image {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW == newW && srcH == newH {
		return img
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		sy := b.Min.Y + y*srcH/newH
		for x := 0; x < newW; x++ {
			sx := b.Min.X + x*srcW/newW
			dst.Set(x, y, img.At(sx, sy))
		}
	}
	return dst
}
