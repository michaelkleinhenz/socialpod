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
	"image/color"
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
	GameID                      string            `json:"gameId"`
	Title                       string            `json:"title"`
	YearPublished               string            `json:"yearPublished,omitempty"`
	MinPlayers                  string            `json:"minPlayers"`
	MaxPlayers                  string            `json:"maxPlayers"`
	MinPlaytime                 string            `json:"minPlaytime"`
	MaxPlaytime                 string            `json:"maxPlaytime"`
	MinAge                      string            `json:"minAge"`
	Designers                   []string          `json:"designers"`
	Artists                     []string          `json:"artists"`
	Publishers                  []string          `json:"publishers"`
	Rating                      string            `json:"rating,omitempty"`
	Weight                      string            `json:"weight,omitempty"`
	ImageBase64                 string            `json:"imageBase64"`
	ImageFilename               string            `json:"imageFilename"`
	Description                 string            `json:"description,omitempty"`
	SuggestedContent            string            `json:"suggestedContent"`
	SuggestedContentByPlatform  map[string]string `json:"suggestedContentByPlatform,omitempty"`
	SuggestedHashtags           []string          `json:"suggestedHashtags,omitempty"`
}

var (
	bggGameIDRe = regexp.MustCompile(`/boardgame[^/]*/(\d+)`)
	htmlTagRe   = regexp.MustCompile(`<[^>]+>`)
	multiNLRe   = regexp.MustCompile(`\n{3,}`)
)

// FetchGame handles GET /api/bgg/fetch?url=...&episodeType=review
func (h *BGGHandler) FetchGame(c *gin.Context) {
	rawURL := c.Query("url")
	if rawURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url parameter is required"})
		return
	}
	episodeType := c.Query("episodeType")

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

	item, err := fetchBGGItem(ctx, gameID, settings.BGGAPIToken)
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

	var designers, artists, publishers, categories, mechanics []string
	for _, link := range item.Links {
		switch link.Type {
		case "boardgamedesigner":
			designers = append(designers, link.Value)
		case "boardgameartist":
			artists = append(artists, link.Value)
		case "boardgamepublisher":
			publishers = append(publishers, link.Value)
		case "boardgamecategory":
			categories = append(categories, link.Value)
		case "boardgamemechanic":
			mechanics = append(mechanics, link.Value)
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
		if data, err := h.downloadAndProcess(ctx, c, imgURL, episodeType); err == nil {
			imageBase64 = base64.StdEncoding.EncodeToString(data)
			imageFilename = fmt.Sprintf("bgg-%s.jpg", gameID)
		}
	}

	baseContent := buildPostContent(title, designers, artists, publishers, item, aiSummary, settings.AILanguage, nil)

	// Generate hashtags for Instagram/Twitter (best-effort; omitted if not configured)
	var suggestedHashtags []string
	if settings.OpenRouterAPIKey != "" {
		if tags, err := h.generateGameHashtags(ctx, title, categories, mechanics, settings); err == nil {
			suggestedHashtags = tags
		}
	}

	// Resolve social handles if enabled for this team and OpenRouter is configured.
	var contentByPlatform map[string]string
	if settings.OpenRouterAPIKey != "" && h.teamHandleLookupEnabled(ctx, c) {
		allNames := append(append(publishers, designers...), artists...)
		handlesByPlatform := h.resolveHandlesWithAI(ctx, allNames, settings)
		if len(handlesByPlatform) > 0 {
			contentByPlatform = make(map[string]string)
			for platform, handles := range handlesByPlatform {
				text := buildPostContent(title, designers, artists, publishers, item, aiSummary, settings.AILanguage, handles)
				if text != baseContent {
					contentByPlatform[platform] = text
				}
			}
			if len(contentByPlatform) == 0 {
				contentByPlatform = nil
			}
		}
	}

	resp := BGGGameResponse{
		GameID:                     gameID,
		Title:                      title,
		YearPublished:              item.YearPub.Value,
		MinPlayers:                 item.MinPlayers.Value,
		MaxPlayers:                 item.MaxPlayers.Value,
		MinPlaytime:                item.MinTime.Value,
		MaxPlaytime:                item.MaxTime.Value,
		MinAge:                     item.MinAge.Value,
		Designers:                  nilSlice(designers),
		Artists:                    nilSlice(artists),
		Publishers:                 nilSlice(publishers),
		Rating:                     trimFloat(item.Stats.Ratings.Average.Value),
		Weight:                     trimFloat(item.Stats.Ratings.Weight.Value),
		ImageBase64:                imageBase64,
		ImageFilename:              imageFilename,
		Description:                description,
		SuggestedContent:           baseContent,
		SuggestedContentByPlatform: contentByPlatform,
		SuggestedHashtags:          suggestedHashtags,
	}
	c.JSON(http.StatusOK, resp)
}

func fetchBGGItem(ctx context.Context, gameID string, token string) (bggItem, error) {
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
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Referer", "https://boardgamegeek.com/")
		req.Header.Set("sec-ch-ua", `"Chromium";v="136", "Google Chrome";v="136", "Not.A/Brand";v="99"`)
		req.Header.Set("sec-ch-ua-mobile", "?0")
		req.Header.Set("sec-ch-ua-platform", `"Windows"`)
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

type bggTeamConfig struct {
	watermark image.Image
	offsetX   int
	offsetY   int
}

func (h *BGGHandler) downloadAndProcess(ctx context.Context, c *gin.Context, imgURL string, episodeType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Referer", "https://boardgamegeek.com/")
	req.Header.Set("sec-ch-ua", `"Chromium";v="136", "Google Chrome";v="136", "Not.A/Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")

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

	cfg := h.loadTeamBGGConfig(ctx, c, episodeType)

	// Letterbox: blurred fill background + scaled-to-fit cover with offset
	const size = 1080
	composed := compositeLetterbox(src, size, cfg.offsetX, cfg.offsetY)

	// Overlay episode-type-specific or BGG watermark if configured
	if cfg.watermark != nil {
		bounds := composed.Bounds()
		wmScaled := resizeImage(cfg.watermark, bounds.Dx(), bounds.Dy())
		dst := image.NewRGBA(bounds)
		draw.Draw(dst, bounds, composed, bounds.Min, draw.Src)
		draw.Draw(dst, bounds, wmScaled, image.Point{}, draw.Over)
		composed = dst
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, composed, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h *BGGHandler) loadTeamBGGConfig(ctx context.Context, c *gin.Context, episodeType string) bggTeamConfig {
	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		return bggTeamConfig{}
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		return bggTeamConfig{}
	}

	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		return bggTeamConfig{}
	}

	cfg := bggTeamConfig{
		offsetX: team.BGGCoverOffsetX,
		offsetY: team.BGGCoverOffsetY,
	}

	// Use episode-type-specific overlay when available, fall back to BGG watermark
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
		overlayID = team.BGGWatermarkID
	}

	if overlayID != nil {
		var wm models.Watermark
		if err := h.DB.Watermarks().FindOne(ctx, bson.M{"_id": *overlayID}).Decode(&wm); err == nil {
			filename := filepath.Base(wm.URL)
			if data, err := os.ReadFile(filepath.Join(h.UploadDir, filename)); err == nil {
				if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
					cfg.watermark = img
				}
			}
		}
	}

	return cfg
}

type bggLabels struct {
	Players                string
	Time                   string
	Min                    string
	Age                    string
	Weight                 string
	BGGRating              string
	Published              string
	Publisher              string
	Designer               string
	Artist                 string
	DescriptionAttribution string
}

var bggLabelsByLanguage = map[string]bggLabels{
	"German": {
		Players: "Spieler", Time: "Dauer", Min: "Min", Age: "Alter",
		Weight: "Komplexität", BGGRating: "BGG-Wertung", Published: "Erschienen",
		Publisher: "Verlag", Designer: "Autor", Artist: "Illustrator",
		DescriptionAttribution: "(Beschreibung des Verlags)",
	},
	"French": {
		Players: "Joueurs", Time: "Durée", Min: "min", Age: "Âge",
		Weight: "Complexité", BGGRating: "Note BGG", Published: "Publié",
		Publisher: "Éditeur", Designer: "Auteur", Artist: "Illustrateur",
		DescriptionAttribution: "(description de l'éditeur)",
	},
	"Spanish": {
		Players: "Jugadores", Time: "Duración", Min: "min", Age: "Edad",
		Weight: "Peso", BGGRating: "Nota BGG", Published: "Publicado",
		Publisher: "Editorial", Designer: "Diseñador", Artist: "Artista",
		DescriptionAttribution: "(descripción del editor)",
	},
	"Italian": {
		Players: "Giocatori", Time: "Durata", Min: "min", Age: "Età",
		Weight: "Complessità", BGGRating: "Valutazione BGG", Published: "Pubblicato",
		Publisher: "Editore", Designer: "Autore", Artist: "Illustratore",
		DescriptionAttribution: "(descrizione dell'editore)",
	},
	"Dutch": {
		Players: "Spelers", Time: "Speeltijd", Min: "min", Age: "Leeftijd",
		Weight: "Complexiteit", BGGRating: "BGG Score", Published: "Gepubliceerd",
		Publisher: "Uitgever", Designer: "Ontwerper", Artist: "Illustrator",
		DescriptionAttribution: "(beschrijving van de uitgever)",
	},
	"Portuguese": {
		Players: "Jogadores", Time: "Duração", Min: "min", Age: "Idade",
		Weight: "Peso", BGGRating: "Nota BGG", Published: "Publicado",
		Publisher: "Editora", Designer: "Designer", Artist: "Artista",
		DescriptionAttribution: "(descrição do editor)",
	},
	"Brazilian Portuguese": {
		Players: "Jogadores", Time: "Duração", Min: "min", Age: "Idade",
		Weight: "Complexidade", BGGRating: "Nota BGG", Published: "Publicado",
		Publisher: "Editora", Designer: "Designer", Artist: "Artista",
		DescriptionAttribution: "(descrição da editora)",
	},
	"Japanese": {
		Players: "プレイ人数", Time: "プレイ時間", Min: "分", Age: "対象年齢",
		Weight: "複雑さ", BGGRating: "BGG評価", Published: "発行年",
		Publisher: "出版社", Designer: "デザイナー", Artist: "アーティスト",
		DescriptionAttribution: "(出版社による説明)",
	},
	"Korean": {
		Players: "플레이어", Time: "플레이 시간", Min: "분", Age: "나이",
		Weight: "복잡도", BGGRating: "BGG 평점", Published: "출판 연도",
		Publisher: "출판사", Designer: "디자이너", Artist: "아티스트",
		DescriptionAttribution: "(출판사의 설명)",
	},
	"Chinese": {
		Players: "玩家人数", Time: "游戏时长", Min: "分钟", Age: "年龄",
		Weight: "复杂度", BGGRating: "BGG评分", Published: "出版年份",
		Publisher: "出版商", Designer: "设计师", Artist: "插画师",
		DescriptionAttribution: "(出版商的描述)",
	},
	"Arabic": {
		Players: "اللاعبون", Time: "الوقت", Min: "دقيقة", Age: "العمر",
		Weight: "التعقيد", BGGRating: "تقييم BGG", Published: "سنة النشر",
		Publisher: "الناشر", Designer: "المصمم", Artist: "الرسام",
		DescriptionAttribution: "(وصف الناشر)",
	},
	"Polish": {
		Players: "Gracze", Time: "Czas", Min: "min", Age: "Wiek",
		Weight: "Złożoność", BGGRating: "Ocena BGG", Published: "Wydano",
		Publisher: "Wydawca", Designer: "Projektant", Artist: "Ilustrator",
		DescriptionAttribution: "(opis wydawcy)",
	},
	"Swedish": {
		Players: "Spelare", Time: "Speltid", Min: "min", Age: "Ålder",
		Weight: "Komplexitet", BGGRating: "BGG-betyg", Published: "Publicerad",
		Publisher: "Förlag", Designer: "Designer", Artist: "Illustratör",
		DescriptionAttribution: "(beskrivning från förlaget)",
	},
	"Norwegian": {
		Players: "Spillere", Time: "Spilletid", Min: "min", Age: "Alder",
		Weight: "Kompleksitet", BGGRating: "BGG-vurdering", Published: "Publisert",
		Publisher: "Utgiver", Designer: "Designer", Artist: "Illustratør",
		DescriptionAttribution: "(beskrivelse fra forlaget)",
	},
	"Danish": {
		Players: "Spillere", Time: "Spilletid", Min: "min", Age: "Alder",
		Weight: "Kompleksitet", BGGRating: "BGG-vurdering", Published: "Udgivet",
		Publisher: "Udgiver", Designer: "Designer", Artist: "Illustratør",
		DescriptionAttribution: "(beskrivelse fra forlaget)",
	},
	"Finnish": {
		Players: "Pelaajat", Time: "Peliaika", Min: "min", Age: "Ikä",
		Weight: "Monimutkaisuus", BGGRating: "BGG-arvosana", Published: "Julkaistu",
		Publisher: "Kustantaja", Designer: "Suunnittelija", Artist: "Taiteilija",
		DescriptionAttribution: "(kustantajan kuvaus)",
	},
}

var defaultBGGLabels = bggLabels{
	Players: "Players", Time: "Time", Min: "min", Age: "Age",
	Weight: "Weight", BGGRating: "BGG Rating", Published: "Published",
	Publisher: "Publisher", Designer: "Designer", Artist: "Artist",
	DescriptionAttribution: "(description from the publisher)",
}

func labelsFor(language string) bggLabels {
	if l, ok := bggLabelsByLanguage[language]; ok {
		return l
	}
	return defaultBGGLabels
}

func (h *BGGHandler) generateGameSummary(ctx context.Context, description, title string, settings models.AppSettings) (string, error) {
	model := settings.OpenRouterModel
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	systemPrompt := "You are a board game expert. Write exactly one sentence that captures what makes this board game unique and fun to play. Be concise and engaging. Do not translate the name of the game. Reply with ONLY the single sentence, no quotes, no extra text."
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

// generateGameHashtags generates 3-5 conservative hashtags for Instagram/Twitter posts.
func (h *BGGHandler) generateGameHashtags(ctx context.Context, title string, categories []string, mechanics []string, settings models.AppSettings) ([]string, error) {
	model := settings.OpenRouterModel
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	systemPrompt := `You are a social media expert for board game content. Generate 3 to 5 hashtags for an Instagram/Twitter post about a board game.

Rules:
- Always include #boardgames
- Include the game title as a hashtag if it forms a clean single-word or well-known tag (e.g. #Carcassonne, #Wingspan)
- Add at most 1-2 thematic hashtags drawn from the categories or mechanics if they are well-known tags (e.g. #cardgame, #familygame, #strategygame)
- Be conservative: 3 strong hashtags beat 5 weak ones
- Do NOT include niche, obscure, or overly specific tags
- Reply with ONLY the hashtags separated by spaces, no other text, no punctuation besides #`

	var inputParts []string
	inputParts = append(inputParts, "Game: "+title)
	if len(categories) > 0 {
		cats := categories
		if len(cats) > 6 {
			cats = cats[:6]
		}
		inputParts = append(inputParts, "Categories: "+strings.Join(cats, ", "))
	}
	if len(mechanics) > 0 {
		mechs := mechanics
		if len(mechs) > 6 {
			mechs = mechs[:6]
		}
		inputParts = append(inputParts, "Mechanics: "+strings.Join(mechs, ", "))
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": strings.Join(inputParts, "\n")},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+settings.OpenRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter returned %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) == 0 {
		return nil, fmt.Errorf("invalid OpenRouter response")
	}

	raw := strings.TrimSpace(result.Choices[0].Message.Content)
	var tags []string
	for _, word := range strings.Fields(raw) {
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			tags = append(tags, word)
		}
	}
	if len(tags) > 5 {
		tags = tags[:5]
	}
	return tags, nil
}

// teamHandleLookupEnabled returns true if the team (or user without a team) has
// handle lookup enabled. When the flag is not set on the team we default to true.
func (h *BGGHandler) teamHandleLookupEnabled(ctx context.Context, c *gin.Context) bool {
	teamIDStr, ok := c.Get("teamId")
	if !ok || teamIDStr.(string) == "" {
		return true
	}
	teamID, err := primitive.ObjectIDFromHex(teamIDStr.(string))
	if err != nil {
		return true
	}
	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		return true
	}
	if team.BGGHandleLookupEnabled == nil {
		return true
	}
	return *team.BGGHandleLookupEnabled
}

// resolveHandlesWithAI looks up handles for each name across all platforms.
// It first checks the local catalog; names not found there are sent to the AI
// in a single batched request. Returns map[platform]map[name]handle.
func (h *BGGHandler) resolveHandlesWithAI(ctx context.Context, names []string, settings models.AppSettings) map[string]map[string]string {
	if len(names) == 0 || settings.OpenRouterAPIKey == "" {
		return nil
	}

	// Dedup names
	seen := map[string]bool{}
	unique := names[:0]
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			unique = append(unique, n)
		}
	}

	// Load catalog from DB
	fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cursor, err := h.DB.PublisherHandles().Find(fetchCtx, bson.M{})
	catalog := map[string]map[string]string{} // name → (platform → handle)
	if err == nil {
		var entries []models.PublisherHandle
		cursor.All(fetchCtx, &entries)
		for _, e := range entries {
			catalog[e.Name] = e.Handles
		}
	}

	// Separate known from unknown names
	result := map[string]map[string]string{} // platform → (name → handle)
	addHandle := func(platform, name, handle string) {
		if handle == "" {
			return
		}
		if result[platform] == nil {
			result[platform] = map[string]string{}
		}
		result[platform][name] = handle
	}

	var unknown []string
	for _, name := range unique {
		if handles, ok := catalog[name]; ok {
			for platform, handle := range handles {
				addHandle(platform, name, handle)
			}
		} else {
			unknown = append(unknown, name)
		}
	}

	// Ask AI for unknown names
	if len(unknown) > 0 {
		aiHandles := h.lookupHandlesViaAI(ctx, unknown, catalog, settings)
		for name, handles := range aiHandles {
			for platform, handle := range handles {
				addHandle(platform, name, handle)
			}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// lookupHandlesViaAI asks the configured model to find social-media handles for
// a list of board-game publisher/designer/artist names. It uses existing catalog
// entries as few-shot examples to improve accuracy.
func (h *BGGHandler) lookupHandlesViaAI(ctx context.Context, names []string, catalog map[string]map[string]string, settings models.AppSettings) map[string]map[string]string {
	model := settings.OpenRouterModel
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	// Build few-shot examples from catalog (up to 5)
	examples := ""
	count := 0
	for name, handles := range catalog {
		if count >= 5 {
			break
		}
		for platform, handle := range handles {
			examples += fmt.Sprintf("  %q -> %s: %q\n", name, platform, handle)
		}
		count++
	}

	platformList := "instagram, bluesky, threads, mastodon, twitter"
	systemPrompt := `You are a board game industry expert. Given a list of board game publisher, designer, or artist names, find their social media handles.
Return ONLY a JSON object where keys are the exact input names and values are objects mapping platform names to handles (with @ prefix).
Only include platforms where you are confident the handle is correct. If you don't know a handle, omit that platform.
Platforms to consider: ` + platformList + `.
Example output format:
{
  "Lookout Games": {"instagram": "@lookoutgames", "bluesky": "@lookout.bsky.social"},
  "Uwe Rosenberg": {"instagram": "@uwerosenberg"}
}`
	if examples != "" {
		systemPrompt += "\n\nKnown handles from our catalog (use as reference):\n" + examples
	}

	namesJSON, _ := json.Marshal(names)
	userMsg := fmt.Sprintf("Find social media handles for these names: %s", string(namesJSON))

	reqBody, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"response_format": map[string]string{"type": "json_object"},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+settings.OpenRouterAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var aiResp struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &aiResp); err != nil || len(aiResp.Choices) == 0 {
		return nil
	}

	var parsed map[string]map[string]string
	if err := json.Unmarshal([]byte(aiResp.Choices[0].Message.Content), &parsed); err != nil {
		return nil
	}
	return parsed
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
	ncWmID := ""
	if team.NewsCreatorWatermarkID != nil {
		ncWmID = team.NewsCreatorWatermarkID.Hex()
	}
	ecWmID := ""
	if team.EpisodeCreatorWatermarkID != nil {
		ecWmID = team.EpisodeCreatorWatermarkID.Hex()
	}
	eoNewsID := ""
	if team.EpisodeOverlayNewsID != nil {
		eoNewsID = team.EpisodeOverlayNewsID.Hex()
	}
	eoReviewID := ""
	if team.EpisodeOverlayReviewID != nil {
		eoReviewID = team.EpisodeOverlayReviewID.Hex()
	}
	eoSpecialID := ""
	if team.EpisodeOverlaySpecialID != nil {
		eoSpecialID = team.EpisodeOverlaySpecialID.Hex()
	}
	handleLookup := true
	if team.BGGHandleLookupEnabled != nil {
		handleLookup = *team.BGGHandleLookupEnabled
	}
	enabledPlugins := team.EnabledPlugins
	if enabledPlugins == nil {
		enabledPlugins = []string{}
	}
	c.JSON(http.StatusOK, gin.H{
		"bggWatermarkId":                wmID,
		"bggCoverOffsetX":               team.BGGCoverOffsetX,
		"bggCoverOffsetY":               team.BGGCoverOffsetY,
		"episodeNewsUrl":                team.EpisodeNewsURL,
		"hasEpisodeNewsBearerToken":     team.EpisodeNewsBearerToken != "",
		"newsCreatorUrl":                team.NewsCreatorURL,
		"hasNewsCreatorBearerToken":     team.NewsCreatorBearerToken != "",
		"newsCreatorWatermarkId":        ncWmID,
		"episodeCreatorUrl":             team.EpisodeCreatorURL,
		"hasEpisodeCreatorBearerToken":  team.EpisodeCreatorBearerToken != "",
		"episodeCreatorWatermarkId":     ecWmID,
		"episodeOverlayNewsId":          eoNewsID,
		"episodeOverlayReviewId":        eoReviewID,
		"episodeOverlaySpecialId":       eoSpecialID,
		"bggHandleLookupEnabled":        handleLookup,
		"enabledPlugins":                enabledPlugins,
	})
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
		BGGWatermarkID              *string `json:"bggWatermarkId"`
		BGGCoverOffsetX             *int    `json:"bggCoverOffsetX"`
		BGGCoverOffsetY             *int    `json:"bggCoverOffsetY"`
		EpisodeNewsURL              *string `json:"episodeNewsUrl"`
		EpisodeNewsBearerToken      *string `json:"episodeNewsBearerToken"`
		NewsCreatorURL              *string `json:"newsCreatorUrl"`
		NewsCreatorBearerToken      *string `json:"newsCreatorBearerToken"`
		NewsCreatorWatermarkID      *string `json:"newsCreatorWatermarkId"`
		EpisodeCreatorURL           *string `json:"episodeCreatorUrl"`
		EpisodeCreatorBearerToken   *string `json:"episodeCreatorBearerToken"`
		EpisodeCreatorWatermarkID   *string `json:"episodeCreatorWatermarkId"`
		EpisodeOverlayNewsID        *string `json:"episodeOverlayNewsId"`
		EpisodeOverlayReviewID      *string `json:"episodeOverlayReviewId"`
		EpisodeOverlaySpecialID     *string `json:"episodeOverlaySpecialId"`
		BGGHandleLookupEnabled      *bool   `json:"bggHandleLookupEnabled"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setFields := bson.M{"updatedAt": time.Now()}
	unsetFields := bson.M{}

	if input.BGGWatermarkID == nil || *input.BGGWatermarkID == "" {
		unsetFields["bggWatermarkId"] = ""
	} else {
		wmID, err := primitive.ObjectIDFromHex(*input.BGGWatermarkID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
			return
		}
		setFields["bggWatermarkId"] = wmID
	}

	if input.BGGCoverOffsetX != nil {
		setFields["bggCoverOffsetX"] = *input.BGGCoverOffsetX
	}
	if input.BGGCoverOffsetY != nil {
		setFields["bggCoverOffsetY"] = *input.BGGCoverOffsetY
	}

	if input.EpisodeNewsURL != nil {
		if *input.EpisodeNewsURL == "" {
			unsetFields["episodeNewsUrl"] = ""
		} else {
			setFields["episodeNewsUrl"] = *input.EpisodeNewsURL
		}
	}
	if input.EpisodeNewsBearerToken != nil {
		if *input.EpisodeNewsBearerToken == "" {
			unsetFields["episodeNewsBearerToken"] = ""
		} else {
			setFields["episodeNewsBearerToken"] = *input.EpisodeNewsBearerToken
		}
	}
	if input.NewsCreatorURL != nil {
		if *input.NewsCreatorURL == "" {
			unsetFields["newsCreatorUrl"] = ""
		} else {
			setFields["newsCreatorUrl"] = *input.NewsCreatorURL
		}
	}
	if input.NewsCreatorBearerToken != nil {
		if *input.NewsCreatorBearerToken == "" {
			unsetFields["newsCreatorBearerToken"] = ""
		} else {
			setFields["newsCreatorBearerToken"] = *input.NewsCreatorBearerToken
		}
	}
	if input.NewsCreatorWatermarkID != nil {
		if *input.NewsCreatorWatermarkID == "" {
			unsetFields["newsCreatorWatermarkId"] = ""
		} else {
			ncWmID, err := primitive.ObjectIDFromHex(*input.NewsCreatorWatermarkID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
				return
			}
			setFields["newsCreatorWatermarkId"] = ncWmID
		}
	}
	if input.EpisodeCreatorURL != nil {
		if *input.EpisodeCreatorURL == "" {
			unsetFields["episodeCreatorUrl"] = ""
		} else {
			setFields["episodeCreatorUrl"] = *input.EpisodeCreatorURL
		}
	}
	if input.EpisodeCreatorBearerToken != nil {
		if *input.EpisodeCreatorBearerToken == "" {
			unsetFields["episodeCreatorBearerToken"] = ""
		} else {
			setFields["episodeCreatorBearerToken"] = *input.EpisodeCreatorBearerToken
		}
	}
	if input.EpisodeCreatorWatermarkID != nil {
		if *input.EpisodeCreatorWatermarkID == "" {
			unsetFields["episodeCreatorWatermarkId"] = ""
		} else {
			ecWmID, err := primitive.ObjectIDFromHex(*input.EpisodeCreatorWatermarkID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
				return
			}
			setFields["episodeCreatorWatermarkId"] = ecWmID
		}
	}
	for _, pair := range []struct {
		input *string
		field string
	}{
		{input.EpisodeOverlayNewsID, "episodeOverlayNewsId"},
		{input.EpisodeOverlayReviewID, "episodeOverlayReviewId"},
		{input.EpisodeOverlaySpecialID, "episodeOverlaySpecialId"},
	} {
		if pair.input != nil {
			if *pair.input == "" {
				unsetFields[pair.field] = ""
			} else {
				oid, err := primitive.ObjectIDFromHex(*pair.input)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid overlay watermark ID"})
					return
				}
				setFields[pair.field] = oid
			}
		}
	}
	if input.BGGHandleLookupEnabled != nil {
		setFields["bggHandleLookupEnabled"] = *input.BGGHandleLookupEnabled
	}

	var updateDoc bson.M
	if len(unsetFields) > 0 {
		updateDoc = bson.M{"$set": setFields, "$unset": unsetFields}
	} else {
		updateDoc = bson.M{"$set": setFields}
	}

	result, err := h.DB.Teams().UpdateOne(ctx, bson.M{"_id": teamID}, updateDoc)
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Settings updated"})
}

// AdminGetTeamSettings returns BGG settings for any team (admin only).
func (h *BGGHandler) AdminGetTeamSettings(c *gin.Context) {
	teamID, err := primitive.ObjectIDFromHex(c.Param("id"))
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
	adminNcWmID := ""
	if team.NewsCreatorWatermarkID != nil {
		adminNcWmID = team.NewsCreatorWatermarkID.Hex()
	}
	adminEcWmID := ""
	if team.EpisodeCreatorWatermarkID != nil {
		adminEcWmID = team.EpisodeCreatorWatermarkID.Hex()
	}
	adminEoNewsID := ""
	if team.EpisodeOverlayNewsID != nil {
		adminEoNewsID = team.EpisodeOverlayNewsID.Hex()
	}
	adminEoReviewID := ""
	if team.EpisodeOverlayReviewID != nil {
		adminEoReviewID = team.EpisodeOverlayReviewID.Hex()
	}
	adminEoSpecialID := ""
	if team.EpisodeOverlaySpecialID != nil {
		adminEoSpecialID = team.EpisodeOverlaySpecialID.Hex()
	}
	adminHandleLookup := true
	if team.BGGHandleLookupEnabled != nil {
		adminHandleLookup = *team.BGGHandleLookupEnabled
	}
	adminEnabledPlugins := team.EnabledPlugins
	if adminEnabledPlugins == nil {
		adminEnabledPlugins = []string{}
	}
	c.JSON(http.StatusOK, gin.H{
		"bggWatermarkId":                wmID,
		"bggCoverOffsetX":               team.BGGCoverOffsetX,
		"bggCoverOffsetY":               team.BGGCoverOffsetY,
		"episodeNewsUrl":                team.EpisodeNewsURL,
		"hasEpisodeNewsBearerToken":     team.EpisodeNewsBearerToken != "",
		"newsCreatorUrl":                team.NewsCreatorURL,
		"hasNewsCreatorBearerToken":     team.NewsCreatorBearerToken != "",
		"newsCreatorWatermarkId":        adminNcWmID,
		"episodeCreatorUrl":             team.EpisodeCreatorURL,
		"hasEpisodeCreatorBearerToken":  team.EpisodeCreatorBearerToken != "",
		"episodeCreatorWatermarkId":     adminEcWmID,
		"episodeOverlayNewsId":          adminEoNewsID,
		"episodeOverlayReviewId":        adminEoReviewID,
		"episodeOverlaySpecialId":       adminEoSpecialID,
		"bggHandleLookupEnabled":        adminHandleLookup,
		"enabledPlugins":                adminEnabledPlugins,
	})
}

// AdminUpdateTeamSettings updates BGG settings for any team (admin only).
func (h *BGGHandler) AdminUpdateTeamSettings(c *gin.Context) {
	teamID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID"})
		return
	}

	var input struct {
		BGGWatermarkID              *string `json:"bggWatermarkId"`
		BGGCoverOffsetX             *int    `json:"bggCoverOffsetX"`
		BGGCoverOffsetY             *int    `json:"bggCoverOffsetY"`
		EpisodeNewsURL              *string `json:"episodeNewsUrl"`
		EpisodeNewsBearerToken      *string `json:"episodeNewsBearerToken"`
		NewsCreatorURL              *string `json:"newsCreatorUrl"`
		NewsCreatorBearerToken      *string `json:"newsCreatorBearerToken"`
		NewsCreatorWatermarkID      *string `json:"newsCreatorWatermarkId"`
		EpisodeCreatorURL           *string `json:"episodeCreatorUrl"`
		EpisodeCreatorBearerToken   *string `json:"episodeCreatorBearerToken"`
		EpisodeCreatorWatermarkID   *string `json:"episodeCreatorWatermarkId"`
		EpisodeOverlayNewsID        *string `json:"episodeOverlayNewsId"`
		EpisodeOverlayReviewID      *string `json:"episodeOverlayReviewId"`
		EpisodeOverlaySpecialID     *string `json:"episodeOverlaySpecialId"`
		BGGHandleLookupEnabled      *bool   `json:"bggHandleLookupEnabled"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setFields := bson.M{"updatedAt": time.Now()}
	unsetFields := bson.M{}

	if input.BGGWatermarkID == nil || *input.BGGWatermarkID == "" {
		unsetFields["bggWatermarkId"] = ""
	} else {
		wmID, err := primitive.ObjectIDFromHex(*input.BGGWatermarkID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
			return
		}
		setFields["bggWatermarkId"] = wmID
	}

	if input.BGGCoverOffsetX != nil {
		setFields["bggCoverOffsetX"] = *input.BGGCoverOffsetX
	}
	if input.BGGCoverOffsetY != nil {
		setFields["bggCoverOffsetY"] = *input.BGGCoverOffsetY
	}

	if input.EpisodeNewsURL != nil {
		if *input.EpisodeNewsURL == "" {
			unsetFields["episodeNewsUrl"] = ""
		} else {
			setFields["episodeNewsUrl"] = *input.EpisodeNewsURL
		}
	}
	if input.EpisodeNewsBearerToken != nil {
		if *input.EpisodeNewsBearerToken == "" {
			unsetFields["episodeNewsBearerToken"] = ""
		} else {
			setFields["episodeNewsBearerToken"] = *input.EpisodeNewsBearerToken
		}
	}
	if input.NewsCreatorURL != nil {
		if *input.NewsCreatorURL == "" {
			unsetFields["newsCreatorUrl"] = ""
		} else {
			setFields["newsCreatorUrl"] = *input.NewsCreatorURL
		}
	}
	if input.NewsCreatorBearerToken != nil {
		if *input.NewsCreatorBearerToken == "" {
			unsetFields["newsCreatorBearerToken"] = ""
		} else {
			setFields["newsCreatorBearerToken"] = *input.NewsCreatorBearerToken
		}
	}
	if input.NewsCreatorWatermarkID != nil {
		if *input.NewsCreatorWatermarkID == "" {
			unsetFields["newsCreatorWatermarkId"] = ""
		} else {
			ncWmID, err := primitive.ObjectIDFromHex(*input.NewsCreatorWatermarkID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
				return
			}
			setFields["newsCreatorWatermarkId"] = ncWmID
		}
	}
	if input.EpisodeCreatorURL != nil {
		if *input.EpisodeCreatorURL == "" {
			unsetFields["episodeCreatorUrl"] = ""
		} else {
			setFields["episodeCreatorUrl"] = *input.EpisodeCreatorURL
		}
	}
	if input.EpisodeCreatorBearerToken != nil {
		if *input.EpisodeCreatorBearerToken == "" {
			unsetFields["episodeCreatorBearerToken"] = ""
		} else {
			setFields["episodeCreatorBearerToken"] = *input.EpisodeCreatorBearerToken
		}
	}
	if input.EpisodeCreatorWatermarkID != nil {
		if *input.EpisodeCreatorWatermarkID == "" {
			unsetFields["episodeCreatorWatermarkId"] = ""
		} else {
			ecWmID, err := primitive.ObjectIDFromHex(*input.EpisodeCreatorWatermarkID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watermark ID"})
				return
			}
			setFields["episodeCreatorWatermarkId"] = ecWmID
		}
	}
	for _, pair := range []struct {
		input *string
		field string
	}{
		{input.EpisodeOverlayNewsID, "episodeOverlayNewsId"},
		{input.EpisodeOverlayReviewID, "episodeOverlayReviewId"},
		{input.EpisodeOverlaySpecialID, "episodeOverlaySpecialId"},
	} {
		if pair.input != nil {
			if *pair.input == "" {
				unsetFields[pair.field] = ""
			} else {
				oid, err := primitive.ObjectIDFromHex(*pair.input)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid overlay watermark ID"})
					return
				}
				setFields[pair.field] = oid
			}
		}
	}
	if input.BGGHandleLookupEnabled != nil {
		setFields["bggHandleLookupEnabled"] = *input.BGGHandleLookupEnabled
	}

	var updateDoc bson.M
	if len(unsetFields) > 0 {
		updateDoc = bson.M{"$set": setFields, "$unset": unsetFields}
	} else {
		updateDoc = bson.M{"$set": setFields}
	}

	result, err := h.DB.Teams().UpdateOne(ctx, bson.M{"_id": teamID}, updateDoc)
	if err != nil || result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Settings updated"})
}

// --- helpers ---

// buildPostContent generates the post text. handles maps BGG name → @handle for the
// target platform; pass nil to get plain names (no handle substitution).
func buildPostContent(title string, designers, artists, publishers []string, item bggItem, aiSummary, language string, handles map[string]string) string {
	l := labelsFor(language)
	var sb strings.Builder

	sb.WriteString(title)
	sb.WriteString("\n\n")

	if aiSummary != "" {
		sb.WriteString(aiSummary)
		sb.WriteString(" ")
		sb.WriteString(l.DescriptionAttribution)
		sb.WriteString("\n\n")
	}

	// Metadata line
	var meta []string
	if item.MinPlayers.Value != "" && item.MaxPlayers.Value != "" {
		if item.MinPlayers.Value == item.MaxPlayers.Value {
			meta = append(meta, fmt.Sprintf("%s: %s", l.Players, item.MinPlayers.Value))
		} else {
			meta = append(meta, fmt.Sprintf("%s: %s-%s", l.Players, item.MinPlayers.Value, item.MaxPlayers.Value))
		}
	}
	if item.MinTime.Value != "" && item.MaxTime.Value != "" {
		if item.MinTime.Value == item.MaxTime.Value {
			meta = append(meta, fmt.Sprintf("%s: %s %s", l.Time, item.MinTime.Value, l.Min))
		} else {
			meta = append(meta, fmt.Sprintf("%s: %s-%s %s", l.Time, item.MinTime.Value, item.MaxTime.Value, l.Min))
		}
	}
	if item.MinAge.Value != "" && item.MinAge.Value != "0" {
		meta = append(meta, fmt.Sprintf("%s: %s+", l.Age, item.MinAge.Value))
	}
	if w := trimFloat(item.Stats.Ratings.Weight.Value); w != "" && w != "0" {
		meta = append(meta, fmt.Sprintf("%s: %s/5", l.Weight, w))
	}
	if len(meta) > 0 {
		sb.WriteString(strings.Join(meta, " | "))
		sb.WriteString("\n")
	}

	if r := trimFloat(item.Stats.Ratings.Average.Value); r != "" && r != "0" {
		sb.WriteString(fmt.Sprintf("%s: %s/10\n", l.BGGRating, r))
	}
	if item.YearPub.Value != "" && item.YearPub.Value != "0" {
		sb.WriteString(fmt.Sprintf("%s: %s\n", l.Published, item.YearPub.Value))
	}
	if len(publishers) > 0 {
		n := min(3, len(publishers))
		sb.WriteString(fmt.Sprintf("%s: %s\n", l.Publisher, formatNamesWithHandles(publishers[:n], handles)))
	}

	sb.WriteString("\n")

	if len(designers) > 0 {
		sb.WriteString(fmt.Sprintf("%s: %s\n", l.Designer, formatNamesWithHandles(designers, handles)))
	}
	if len(artists) > 0 {
		n := min(3, len(artists))
		sb.WriteString(fmt.Sprintf("%s: %s\n", l.Artist, formatNamesWithHandles(artists[:n], handles)))
	}

	return strings.TrimRight(sb.String(), "\n")
}

// formatNamesWithHandles returns names joined with ", ", substituting "@handle (name)"
// where a handle exists for that name.
func formatNamesWithHandles(names []string, handles map[string]string) string {
	if len(handles) == 0 {
		return strings.Join(names, ", ")
	}
	parts := make([]string, len(names))
	for i, name := range names {
		if h, ok := handles[name]; ok && h != "" {
			parts[i] = h + " (" + name + ")"
		} else {
			parts[i] = name
		}
	}
	return strings.Join(parts, ", ")
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

// compositeLetterbox scales src to fit within size×size (preserving aspect ratio),
// places it centered with the given pixel offset, and fills the background with
// a blurred, scaled-to-fill version of src.
func compositeLetterbox(src image.Image, size, offsetX, offsetY int) image.Image {
	b := src.Bounds()
	srcW, srcH := b.Dx(), b.Dy()

	scaleX := float64(size) / float64(srcW)
	scaleY := float64(size) / float64(srcH)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}
	// Reduce by ~17.5% so the cover stays fully in frame even when an offset is applied.
	scale *= 0.825
	fitW := int(float64(srcW) * scale)
	fitH := int(float64(srcH) * scale)
	if fitW < 1 {
		fitW = 1
	}
	if fitH < 1 {
		fitH = 1
	}

	coverScaled := resizeImage(src, fitW, fitH)
	bg := blurredBackground(src, size, size)

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(dst, dst.Bounds(), bg, image.Point{}, draw.Src)

	x := (size-fitW)/2 + offsetX
	y := (size-fitH)/2 + offsetY
	coverRect := image.Rect(x, y, x+fitW, y+fitH)
	draw.Draw(dst, coverRect, coverScaled, image.Point{}, draw.Over)

	return dst
}

// blurredBackground scales src to fill w×h (cropping edges), then blurs it.
// Three passes of box blur approximate a Gaussian blur for a smooth result.
func blurredBackground(src image.Image, w, h int) image.Image {
	smallW, smallH := w/4, h/4
	if smallW < 1 {
		smallW = 1
	}
	if smallH < 1 {
		smallH = 1
	}
	small := scaleToFill(src, smallW, smallH)
	blurred := separableBoxBlur(small, 12)
	blurred = separableBoxBlur(blurred, 12)
	blurred = separableBoxBlur(blurred, 12)
	return resizeImage(blurred, w, h)
}

// scaleToFill scales src so it covers the entire w×h area (no empty bars),
// then center-crops to exactly w×h.
func scaleToFill(src image.Image, w, h int) image.Image {
	b := src.Bounds()
	srcW, srcH := b.Dx(), b.Dy()

	scaleX := float64(w) / float64(srcW)
	scaleY := float64(h) / float64(srcH)
	scale := scaleX
	if scaleY > scaleX {
		scale = scaleY
	}
	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	scaled := resizeImage(src, newW, newH)
	ox := (newW - w) / 2
	oy := (newH - h) / 2
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), scaled, image.Point{ox, oy}, draw.Src)
	return dst
}

// separableBoxBlur applies a two-pass (horizontal then vertical) box blur.
func separableBoxBlur(src image.Image, radius int) image.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, src, bounds.Min, draw.Src)

	d := uint32(2*radius + 1)

	// Horizontal pass
	tmp := image.NewRGBA(bounds)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b uint32
			for dx := -radius; dx <= radius; dx++ {
				nx := x + dx
				if nx < 0 {
					nx = 0
				} else if nx >= w {
					nx = w - 1
				}
				c := rgba.RGBAAt(bounds.Min.X+nx, bounds.Min.Y+y)
				r += uint32(c.R)
				g += uint32(c.G)
				b += uint32(c.B)
			}
			tmp.SetRGBA(bounds.Min.X+x, bounds.Min.Y+y, color.RGBA{
				R: uint8(r / d), G: uint8(g / d), B: uint8(b / d), A: 255,
			})
		}
	}

	// Vertical pass
	dst := image.NewRGBA(bounds)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b uint32
			for dy := -radius; dy <= radius; dy++ {
				ny := y + dy
				if ny < 0 {
					ny = 0
				} else if ny >= h {
					ny = h - 1
				}
				c := tmp.RGBAAt(bounds.Min.X+x, bounds.Min.Y+ny)
				r += uint32(c.R)
				g += uint32(c.G)
				b += uint32(c.B)
			}
			dst.SetRGBA(bounds.Min.X+x, bounds.Min.Y+y, color.RGBA{
				R: uint8(r / d), G: uint8(g / d), B: uint8(b / d), A: 255,
			})
		}
	}
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
