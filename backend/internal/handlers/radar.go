package handlers

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"
	"socialmedia/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	_ "golang.org/x/image/webp" // news images may be WebP
)

// RadarHandler builds the "Radar der Woche" carousel from news drafts that
// were marked for it.
type RadarHandler struct {
	DB        *database.MongoDB
	UploadDir string
}

// A carousel holds at most ten slides: the cover, the games and the closing.
const maxRadarItems = 8

const (
	defaultRadarCaptionIntro = "📡 Radar der Woche: die Ankündigungen, die ihr kennen solltet."
	defaultRadarCaptionOutro = "Schickt das eurer Spielerunde! Und: Welche davon landet auf eurer Liste? 👇\n\n#brettspiele #brettspielneuheiten #boardgames"
	defaultRadarClosingText  = "Welche Neuheit interessiert euch am meisten? Schreibt es in die Kommentare!"
)

// radarQueueFilter selects the drafts marked for the Radar. Posted and open
// drafts both count — the Radar sums up news that usually went out on its own
// already. Whether a draft is still available depends on its Radar post, which
// radarQueueEntries decides.
func radarQueueFilter(c *gin.Context) bson.M {
	filter := newsDraftScopeFilter(c)
	filter["radar"] = true
	return filter
}

// RadarPostRef is the post that carries a queued draft's Radar slide.
type RadarPostRef struct {
	ID          string            `json:"id"`
	Status      models.PostStatus `json:"status"`
	ScheduledAt time.Time         `json:"scheduledAt"`
}

// RadarQueueEntry is a news draft marked for the Radar, with the post that
// already carries its slide, if any.
type RadarQueueEntry struct {
	models.NewsDraft
	RadarPost *RadarPostRef `json:"radarPost,omitempty"`
}

// radarQueueEntries keeps the drafts that are still available for a Radar:
// those no post carries yet, and those whose post has not gone out (marked
// with that post). A draft whose post was published — or that post is gone,
// which only happens to published posts — was used and is left out.
func radarQueueEntries(drafts []models.NewsDraft, posts map[primitive.ObjectID]models.Post) []RadarQueueEntry {
	out := make([]RadarQueueEntry, 0, len(drafts))
	for _, d := range drafts {
		if d.RadarPostID == nil {
			out = append(out, RadarQueueEntry{NewsDraft: d})
			continue
		}
		p, ok := posts[*d.RadarPostID]
		if !ok || p.Status == models.PostStatusPublished {
			continue
		}
		out = append(out, RadarQueueEntry{NewsDraft: d, RadarPost: &RadarPostRef{ID: p.ID.Hex(), Status: p.Status, ScheduledAt: p.ScheduledAt}})
	}
	return out
}

// Queue lists the news drafts available for a Radar post, oldest first,
// including those already in a Radar post that has not gone out yet.
// GET /api/radar/queue
func (h *RadarHandler) Queue(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cursor, err := h.DB.NewsDrafts().Find(ctx, radarQueueFilter(c), options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load the Radar queue"})
		return
	}
	var drafts []models.NewsDraft
	if err := cursor.All(ctx, &drafts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode the Radar queue"})
		return
	}

	postIDs := []primitive.ObjectID{}
	for _, d := range drafts {
		if d.RadarPostID != nil {
			postIDs = append(postIDs, *d.RadarPostID)
		}
	}
	posts := map[primitive.ObjectID]models.Post{}
	if len(postIDs) > 0 {
		pc, err := h.DB.Posts().Find(ctx, bson.M{"_id": bson.M{"$in": postIDs}},
			options.Find().SetProjection(bson.M{"status": 1, "scheduledAt": 1}))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load the Radar posts"})
			return
		}
		var found []models.Post
		if err := pc.All(ctx, &found); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode the Radar posts"})
			return
		}
		for _, p := range found {
			posts[p.ID] = p
		}
	}
	c.JSON(http.StatusOK, radarQueueEntries(drafts, posts))
}

// Remove takes a news draft off the Radar by clearing its "Radar Post?" mark.
// The draft itself stays; a Radar post that already carries its slide keeps it.
// DELETE /api/radar/queue/:id
func (h *RadarHandler) Remove(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	filter := newsDraftScopeFilter(c)
	filter["_id"] = id
	res, err := h.DB.NewsDrafts().UpdateOne(ctx, filter, bson.M{"$set": bson.M{"radar": false}})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove the news from the Radar"})
		return
	}
	if res.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "News draft not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Removed from the Radar"})
}

// SetRelease sets a queued draft's "Kommt es auf Deutsch?" badge. Slides
// already rendered into a Radar post keep the badge they were made with.
// PUT /api/radar/queue/:id {"germanRelease": "ja"|"nein"|"offen"}
func (h *RadarHandler) SetRelease(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
		return
	}
	var input struct {
		GermanRelease string `json:"germanRelease"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	release := normalizeGermanRelease(input.GermanRelease)
	if release == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "germanRelease must be ja, nein or offen"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	filter := radarQueueFilter(c)
	filter["_id"] = id
	res, err := h.DB.NewsDrafts().UpdateOne(ctx, filter, bson.M{"$set": bson.M{"germanRelease": release, "updatedAt": time.Now()}})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update the news"})
		return
	}
	if res.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "News draft not found on the Radar"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"germanRelease": release})
}

// RadarContent is a generated Radar carousel, ready to drop into a post.
type RadarContent struct {
	ImageURLs []string `json:"imageUrls"`
	Caption   string   `json:"caption"`
	UserTags  []string `json:"userTags"`
	DraftIDs  []string `json:"draftIds"`
}

// Generate renders the Radar carousel for the given queued drafts (in the
// given order) and returns the slide URLs and a caption. Nothing is posted and
// the drafts stay queued until a post carrying the slides is saved.
// POST /api/radar/generate {"draftIds": [...]}
func (h *RadarHandler) Generate(c *gin.Context) {
	var input struct {
		DraftIDs []string `json:"draftIds"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(input.DraftIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pick at least one news item for the Radar"})
		return
	}
	if len(input.DraftIDs) > maxRadarItems {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("A Radar holds at most %d news items (a carousel has ten slides)", maxRadarItems)})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	ids := make([]primitive.ObjectID, 0, len(input.DraftIDs))
	seen := map[primitive.ObjectID]bool{}
	for _, s := range input.DraftIDs {
		id, err := primitive.ObjectIDFromHex(s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid draft ID"})
			return
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	filter := newsDraftScopeFilter(c)
	filter["_id"] = bson.M{"$in": ids}
	cursor, err := h.DB.NewsDrafts().Find(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load news drafts"})
		return
	}
	var found []models.NewsDraft
	if err := cursor.All(ctx, &found); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode news drafts"})
		return
	}
	byID := map[primitive.ObjectID]models.NewsDraft{}
	for _, d := range found {
		byID[d.ID] = d
	}
	drafts := make([]models.NewsDraft, 0, len(ids))
	for _, id := range ids {
		d, ok := byID[id]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "A picked news draft no longer exists"})
			return
		}
		drafts = append(drafts, d)
	}

	settings := defaultGrowthSettings(nil)
	if teamID := teamIDFrom(c); teamID != nil {
		var team models.Team
		if h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team) == nil {
			settings = defaultGrowthSettings(team.Growth)
		}
	}

	content, err := h.render(ctx, drafts, *settings.Radar, time.Now())
	if err != nil {
		log.Printf("Radar generation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build the Radar slides: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, content)
}

// render draws and stores the slides and composes the caption.
func (h *RadarHandler) render(ctx context.Context, drafts []models.NewsDraft, radar models.RadarSettings, now time.Time) (*RadarContent, error) {
	colors := services.ParseRadarColors(services.RadarColorHexes{
		Accent:     radar.AccentColor,
		Background: radar.BackgroundColor,
		Panel:      radar.PanelColor,
		Text:       radar.TextColor,
		Muted:      radar.MutedColor,
		BadgeYes:   radar.BadgeYesColor,
		BadgeNo:    radar.BadgeNoColor,
		BadgeOpen:  radar.BadgeOpenColor,
	})
	fonts := h.loadFont(ctx, radar.FontURL)
	var slides [][]byte

	_, week := now.ISOWeek()
	subtitle := fmt.Sprintf("KW %d · %d %s", week, len(drafts), pluralDE(len(drafts), "Ankündigung", "Ankündigungen"))
	cover, err := services.RenderRadarCover(h.loadImage(ctx, radar.CoverImageURL), radar.Title, subtitle, colors, fonts)
	if err != nil {
		return nil, err
	}
	slides = append(slides, cover)

	for i, d := range drafts {
		var img image.Image
		if len(d.ImageURLs) > 0 {
			img = h.loadImage(ctx, d.ImageURLs[0])
		}
		slide, err := services.RenderRadarItem(services.RadarItemSlide{
			Index:         i + 1,
			Total:         len(drafts),
			Title:         radarItemTitle(d),
			Image:         img,
			GermanRelease: d.GermanRelease,
		}, radar.Title, radar.BadgeQuestion, colors, fonts)
		if err != nil {
			return nil, err
		}
		slides = append(slides, slide)
	}

	closingText := radar.ClosingText
	if closingText == "" {
		closingText = defaultRadarClosingText
	}
	closing, err := services.RenderRadarClosing(h.loadImage(ctx, radar.ClosingImageURL), closingText, colors, fonts)
	if err != nil {
		return nil, err
	}
	slides = append(slides, closing)

	content := &RadarContent{ImageURLs: []string{}, UserTags: []string{}, DraftIDs: []string{}}
	stamp := now.UnixNano()
	for i, data := range slides {
		url, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("radar-%d-%02d.jpg", stamp, i), "image/jpeg", data)
		if err != nil {
			return nil, err
		}
		content.ImageURLs = append(content.ImageURLs, url)
	}
	for _, d := range drafts {
		content.DraftIDs = append(content.DraftIDs, d.ID.Hex())
	}
	content.Caption, content.UserTags = radarCaption(drafts, radar)
	return content, nil
}

func pluralDE(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// radarItemTitle is the line a game's slide and caption entry show: the news
// tagline, or the first line of its post text.
func radarItemTitle(d models.NewsDraft) string {
	if t := strings.TrimSpace(d.NewsTagline); t != "" {
		return t
	}
	return firstLine(stripHandles(firstLine(instagramText(d), 200)), 60)
}

func instagramText(d models.NewsDraft) string {
	if d.ContentOverrides != nil && strings.TrimSpace(d.ContentOverrides["instagram"]) != "" {
		return d.ContentOverrides["instagram"]
	}
	return d.Content
}

// igMentionRe finds @usernames, not the "@" inside an e-mail address.
var igMentionRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9._@])@([A-Za-z0-9._]{1,30})`)

// extractIGHandles returns the @usernames a text mentions, in order, without
// duplicates. A trailing full stop belongs to the sentence, not the name.
func extractIGHandles(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range igMentionRe.FindAllStringSubmatch(text, -1) {
		h := strings.TrimRight(m[1], ".")
		key := strings.ToLower(h)
		if h == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, h)
	}
	return out
}

func stripHandles(text string) string {
	return strings.Join(strings.Fields(igMentionRe.ReplaceAllString(text, " ")), " ")
}

func germanReleaseLabel(v string) string {
	switch v {
	case "ja":
		return "Ja"
	case "nein":
		return "Nein"
	case "offen":
		return "Offen"
	}
	return ""
}

// radarCaption writes the carousel caption — intro, one numbered line per
// game with its badge answer and handles, outro — and collects every handle
// for tagging.
func radarCaption(drafts []models.NewsDraft, radar models.RadarSettings) (string, []string) {
	intro := radar.CaptionIntro
	if intro == "" {
		intro = defaultRadarCaptionIntro
	}
	outro := radar.CaptionOutro
	if outro == "" {
		outro = defaultRadarCaptionOutro
	}

	var b strings.Builder
	b.WriteString(intro)
	b.WriteString("\n\n")
	var tags []string
	seen := map[string]bool{}
	for i, d := range drafts {
		fmt.Fprintf(&b, "%d. %s", i+1, radarItemTitle(d))
		if label := germanReleaseLabel(d.GermanRelease); label != "" {
			fmt.Fprintf(&b, " · %s %s", radar.BadgeQuestion, label)
		}
		handles := extractIGHandles(instagramText(d))
		if len(handles) > 0 {
			b.WriteString(" (")
			for j, h := range handles {
				if j > 0 {
					b.WriteString(" ")
				}
				b.WriteString("@" + h)
				if !seen[strings.ToLower(h)] {
					seen[strings.ToLower(h)] = true
					tags = append(tags, h)
				}
			}
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(outro)
	if len(tags) > 20 {
		tags = tags[:20]
	}
	if tags == nil {
		tags = []string{}
	}
	return b.String(), tags
}

// loadImage decodes an image referenced by URL: one of our uploads (disk
// first, then the uploads collection) or, failing that, a remote URL. Any
// failure yields nil, and the slide is drawn without it.
func (h *RadarHandler) loadImage(ctx context.Context, url string) image.Image {
	if url == "" {
		return nil
	}
	data, err := readImageBytes(ctx, h.DB, h.UploadDir, url)
	if err != nil {
		log.Printf("Radar: could not load %s: %v", url, err)
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		log.Printf("Radar: could not decode %s: %v", url, err)
		return nil
	}
	if o := services.ReadExifOrientation(data); o >= 2 && o <= 8 {
		img = services.ApplyOrientation(img, o)
	}
	return img
}

// loadFont reads the team's uploaded Radar font. Any failure yields the
// built-in fonts, so a broken upload never stops a Radar from being built.
func (h *RadarHandler) loadFont(ctx context.Context, url string) services.RadarFonts {
	if url == "" {
		return services.RadarFonts{}
	}
	data, err := readImageBytes(ctx, h.DB, h.UploadDir, url)
	if err != nil {
		log.Printf("Radar: could not load font %s: %v", url, err)
		return services.RadarFonts{}
	}
	fonts, err := services.ParseRadarFont(data)
	if err != nil {
		log.Printf("Radar: could not parse font %s: %v", url, err)
		return services.RadarFonts{}
	}
	return fonts
}

func readImageBytes(ctx context.Context, db *database.MongoDB, uploadDir, url string) ([]byte, error) {
	if name := uploadFilename(url); name != "" {
		if data, err := os.ReadFile(filepath.Join(uploadDir, name)); err == nil {
			return data, nil
		}
		var up models.Upload
		if err := db.Uploads().FindOne(ctx, bson.M{"filename": name}).Decode(&up); err != nil {
			return nil, err
		}
		return up.Data, nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("unsupported image URL")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 15*1024*1024))
}

// storeUploadBytes saves generated image bytes like any upload: on disk and
// in the uploads collection.
func storeUploadBytes(ctx context.Context, db *database.MongoDB, uploadDir, filename, contentType string, data []byte) (string, error) {
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(uploadDir, filename), data, 0o644); err != nil {
		return "", err
	}
	if _, err := db.Uploads().InsertOne(ctx, models.Upload{
		Filename:    filename,
		ContentType: contentType,
		Data:        data,
		Size:        int64(len(data)),
		CreatedAt:   time.Now(),
	}); err != nil {
		return "", err
	}
	return uploadURLPrefix + filename, nil
}

// linkRadarDrafts records that a saved post carries the Radar slides of the
// given drafts, which takes them out of the Radar queue, and puts back any
// draft the post carried before but no longer does. scope limits the drafts to
// the caller's own.
func linkRadarDrafts(ctx context.Context, db *database.MongoDB, scope bson.M, postID primitive.ObjectID, draftIDs []primitive.ObjectID) {
	release := bson.M{"radarPostId": postID}
	for k, v := range scope {
		release[k] = v
	}
	if len(draftIDs) > 0 {
		release["_id"] = bson.M{"$nin": draftIDs}
	}
	if _, err := db.NewsDrafts().UpdateMany(ctx, release, bson.M{"$unset": bson.M{"radarPostId": ""}}); err != nil {
		log.Printf("Radar: failed to release drafts of post %s: %v", postID.Hex(), err)
	}
	if len(draftIDs) == 0 {
		return
	}
	claim := bson.M{"_id": bson.M{"$in": draftIDs}}
	for k, v := range scope {
		claim[k] = v
	}
	if _, err := db.NewsDrafts().UpdateMany(ctx, claim, bson.M{"$set": bson.M{"radarPostId": postID}}); err != nil {
		log.Printf("Radar: failed to link drafts to post %s: %v", postID.Hex(), err)
	}
}

// releaseRadarDrafts returns a deleted post's drafts to the Radar queue —
// unless the post went out, in which case they were used.
func releaseRadarDrafts(ctx context.Context, db *database.MongoDB, post models.Post) {
	if len(post.RadarDraftIDs) == 0 || post.Status == models.PostStatusPublished {
		return
	}
	if _, err := db.NewsDrafts().UpdateMany(ctx, bson.M{"radarPostId": post.ID}, bson.M{"$unset": bson.M{"radarPostId": ""}}); err != nil {
		log.Printf("Radar: failed to release drafts of deleted post %s: %v", post.ID.Hex(), err)
	}
}

func parseObjectIDs(ids []string) []primitive.ObjectID {
	out := make([]primitive.ObjectID, 0, len(ids))
	for _, s := range ids {
		if id, err := primitive.ObjectIDFromHex(s); err == nil {
			out = append(out, id)
		}
	}
	return out
}
