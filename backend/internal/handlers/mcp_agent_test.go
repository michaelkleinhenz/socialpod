package handlers

import (
	"encoding/json"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Every tool tools/list advertises must be handled by callTool.
func TestEveryMCPToolIsDispatched(t *testing.T) {
	src, err := os.ReadFile("mcp.go")
	if err != nil {
		t.Fatal(err)
	}
	h := &MCPHandler{}
	seen := map[string]bool{}
	for _, tool := range h.toolDefinitions() {
		if seen[tool.Name] {
			t.Errorf("tool %s is defined twice", tool.Name)
		}
		seen[tool.Name] = true
		if !strings.Contains(string(src), `case "`+tool.Name+`":`) {
			t.Errorf("tool %s is listed but callTool has no case for it", tool.Name)
		}
		if tool.InputSchema["type"] != "object" {
			t.Errorf("tool %s: input schema must be an object", tool.Name)
		}
	}
}

// MCP exposes user features only: none of the agent tools may reach a team
// admin or admin handler.
func TestAgentToolsStayUserLevel(t *testing.T) {
	for _, file := range []string{"mcp_agent.go", "mcp_images.go", "mcp_sources.go", "mcp_capture.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"UpdateSettings", "SweepUploads", "/api/team/", "/api/admin/"} {
			if strings.Contains(string(src), banned) {
				t.Errorf("%s uses %s, which is not a user-level feature", file, banned)
			}
		}
	}
}

func TestSourceKey(t *testing.T) {
	cases := []struct{ a, b string }{
		{"https://www.example.com/news/item/?utm_source=x&id=3", "http://example.com/news/item?id=3"},
		{"https://example.com/a?b=2&a=1#top", "example.com/a?a=1&b=2"},
		{"https://boardgamegeek.com/boardgame/266192/wingspan", "https://www.boardgamegeek.com/boardgame/266192"},
		{"https://m.example.com/x?fbclid=abc", "https://example.com/x/"},
	}
	for _, tc := range cases {
		ka, kb := sourceKey(tc.a), sourceKey(tc.b)
		if ka == "" || ka != kb {
			t.Errorf("sourceKey(%q) = %q, sourceKey(%q) = %q; want equal", tc.a, ka, tc.b, kb)
		}
	}
	if k := sourceKey("https://boardgamegeek.com/boardgame/266192/wingspan"); k != "bgg:266192" {
		t.Errorf("BGG key = %q", k)
	}
	if sourceKey("https://example.com/a") == sourceKey("https://example.com/b") {
		t.Error("different pages share a key")
	}
	for _, bad := range []string{"", "ftp://example.com/x", "mailto:a@b.c"} {
		if k := sourceKey(bad); k != "" {
			t.Errorf("sourceKey(%q) = %q, want empty", bad, k)
		}
	}
}

func TestMCPOrigin(t *testing.T) {
	cases := map[string]string{"": "mcp", "agent": "agent", "Agent": "agent", "content-bot_2": "content-bot_2", "bad origin!": "mcp", strings.Repeat("a", 40): "mcp"}
	for in, want := range cases {
		if got := mcpOrigin(map[string]any{"origin": in}); got != want {
			t.Errorf("mcpOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenSlots(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	// Monday 2026-10-12.
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, loc)
	now := start.Add(8 * time.Hour)
	plan := []models.WeeklyPlanSlot{
		{ID: "a", Weekday: 0, Time: "07:00", Format: "reel"}, // already past
		{ID: "b", Weekday: 0, Time: "18:00", Format: "carousel"},
		{ID: "c", Weekday: 0, Time: "12:00", Format: "any"},
		{ID: "d", Weekday: 2, Format: "image"},
	}
	posts := []models.Post{
		// A carousel on Monday afternoon fills the carousel slot, not "any".
		{PostType: models.PostTypePost, ImageURLs: []string{"/a.jpg", "/b.jpg"}, ScheduledAt: start.Add(17 * time.Hour)},
	}
	got := openSlots(plan, posts, start, 7, now, loc)
	ids := []string{}
	for _, s := range got {
		ids = append(ids, s.SlotID)
	}
	if strings.Join(ids, ",") != "c,d" {
		t.Fatalf("open slots = %v, want c,d", ids)
	}
	if got[0].At != start.Add(12*time.Hour).Format(time.RFC3339) {
		t.Errorf("slot c at %s", got[0].At)
	}
	// A slot without time is placed at noon.
	if got[1].Date != "2026-10-14" || !strings.Contains(got[1].At, "T12:00:00") {
		t.Errorf("slot d = %+v", got[1])
	}
	// Stories never fill an "any" slot.
	story := []models.Post{{PostType: models.PostTypeStory, ScheduledAt: start.Add(12 * time.Hour)}}
	if s := openSlots(plan[2:3], story, start, 1, now, loc); len(s) != 1 {
		t.Errorf("a story filled an any slot")
	}
}

func TestCanvasSize(t *testing.T) {
	cases := []struct {
		format       string
		w, h, bw, bh int
		ww, wh       int
	}{
		{"", 0, 0, 0, 0, 1080, 1350},
		{"1:1", 0, 0, 0, 0, 1080, 1080},
		{"9:16", 0, 0, 0, 0, 1080, 1920},
		{"16:9", 0, 0, 0, 0, 1920, 1080},
		{"original", 0, 0, 2000, 1000, 1080, 540},
		{"original", 0, 0, 1000, 2000, 540, 1080},
		{"4:5", 800, 0, 0, 0, 800, 1000},
		{"4:5", 500, 700, 0, 0, 500, 700},
	}
	for _, tc := range cases {
		w, h, err := canvasSize(tc.format, tc.w, tc.h, tc.bw, tc.bh)
		if err != nil || w != tc.ww || h != tc.wh {
			t.Errorf("canvasSize(%q,%d,%d,%d,%d) = %d,%d,%v; want %d,%d", tc.format, tc.w, tc.h, tc.bw, tc.bh, w, h, err, tc.ww, tc.wh)
		}
	}
	if _, _, err := canvasSize("3:7", 0, 0, 0, 0); err == nil {
		t.Error("unknown format accepted")
	}
}

func TestMediaListFilter(t *testing.T) {
	scope := bson.M{"teamId": "t"}
	f := mediaListFilter(scope, "wingspan box", []string{"#Cover", "cover", " art "}, "")
	if f["teamId"] != "t" {
		t.Error("scope lost")
	}
	and, _ := f["$and"].([]bson.M)
	if len(and) != 3 {
		t.Fatalf("want 2 word clauses and 1 tag clause, got %v", and)
	}
	tags := and[2]["tags"].(bson.M)["$all"].([]string)
	if strings.Join(tags, ",") != "cover,art" {
		t.Errorf("tags = %v", tags)
	}
	if _, ok := scope["$and"]; ok {
		t.Error("scope was modified")
	}
	if f := mediaListFilter(scope, "", nil, ""); f["$and"] != nil || f["contentType"] != nil {
		t.Error("empty search should not filter")
	}
	if f := mediaListFilter(scope, "", nil, "Font"); f["contentType"] != (primitive.Regex{Pattern: "^font/"}) {
		t.Errorf("kind font: contentType = %v", f["contentType"])
	}
	if f := mediaListFilter(scope, "", nil, "audio"); f["contentType"] != (primitive.Regex{Pattern: "^audio/"}) {
		t.Errorf("kind audio: contentType = %v", f["contentType"])
	}
	if f := mediaListFilter(scope, "", nil, "spreadsheet"); f["contentType"] != nil {
		t.Error("unknown kind should not filter")
	}
	if f := mediaListFilter(scope, "", nil, "watermark"); f["watermark"] != true || f["contentType"] != nil {
		t.Errorf("kind watermark: filter = %v", f)
	}
}

func TestNormalizeColor(t *testing.T) {
	for in, want := range map[string]string{
		"#1A2b3C":   "#1a2b3c",
		"1a2b3c":    "#1a2b3c",
		" #abc ":    "#aabbcc",
		"#1a2b3cff": "#1a2b3c",
		"#1a2b3c80": "#1a2b3c80",
		"#ffffffff": "#ffffff",
	} {
		if got, err := normalizeColor(in); err != nil || got != want {
			t.Errorf("normalizeColor(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "red", "#12", "#12345", "#1234567", "#gggggg"} {
		if _, err := normalizeColor(bad); err == nil {
			t.Errorf("normalizeColor(%q) accepted", bad)
		}
	}
	if f := mediaListFilter(bson.M{}, "", nil, "color"); f["contentType"] != (primitive.Regex{Pattern: "^color/"}) {
		t.Errorf("kind color: contentType = %v", f["contentType"])
	}
}

func TestCheckCaption(t *testing.T) {
	good := "Der neue Feld: kaufen oder warten?\n\nSchick das deiner Spielerunde, die immer zu spät kommt.\nWas meint ihr?\n\n#brettspiele #kennerspiel #spielessen"
	checks := checkCaption(good, nil, 1)
	for _, c := range checks {
		if c.Level != "ok" {
			t.Errorf("good caption: %s = %s (%s)", c.ID, c.Level, c.Hint)
		}
	}
	if len(checks) != 5 {
		t.Errorf("%d checks without bluesky text, want 5", len(checks))
	}

	bad := "#nur #hashtags"
	byID := map[string]string{}
	same := bad
	for _, c := range checkCaption(bad, &same, 0) {
		byID[c.ID] = c.Level
	}
	want := map[string]string{"hook": "warn", "question": "missing", "share": "missing", "hashtags": "missing", "tagged": "missing", "mirror": "warn"}
	for id, lvl := range want {
		if byID[id] != lvl {
			t.Errorf("bad caption: %s = %s, want %s", id, byID[id], lvl)
		}
	}
}

func TestCallREST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("teamId", "abc")
	handler := func(rc *gin.Context) {
		var in struct{ X int }
		rc.ShouldBindJSON(&in)
		team, _ := rc.Get("teamId")
		if in.X < 0 {
			rc.JSON(http.StatusBadRequest, gin.H{"error": "negative"})
			return
		}
		rc.JSON(http.StatusOK, gin.H{"team": team, "x": in.X})
	}
	status, out, err := callREST(c, handler, http.MethodPost, "/x", map[string]int{"X": 3})
	if err != nil || status != 200 || out["team"] != "abc" || out["x"] != float64(3) {
		t.Fatalf("callREST = %d %v %v", status, out, err)
	}
	res, isErr := restResult(callREST(c, handler, http.MethodPost, "/x", map[string]int{"X": -1}))
	if !isErr || res.(map[string]any)["error"] != "negative" {
		t.Errorf("error answer = %v, %v", res, isErr)
	}
}

func TestIsFullCanvasOverlay(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 1080, 1080))
	opaque := image.NewRGBA(image.Rect(0, 0, 1080, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1080; x++ {
			opaque.Set(x, y, color.RGBA{0, 0, 0, 255})
			if x < 100 || y > 900 {
				frame.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}
	if !isFullCanvasOverlay(frame) {
		t.Error("frame with a transparent middle not recognised")
	}
	if isFullCanvasOverlay(opaque) {
		t.Error("opaque image counted as overlay")
	}
	if isFullCanvasOverlay(image.NewRGBA(image.Rect(0, 0, 200, 200))) {
		t.Error("small logo counted as full-canvas overlay")
	}
}

// A tools/call over JSON-RPC answers with a text block (and no stray image
// fields) for a tool that needs no database.
func TestCheckCaptionOverJSONRPC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &MCPHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"check_instagram_caption","arguments":{"text":"Kurz","collaborators":["verlag"]}}}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/mcp", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Handle(c)
	var resp struct {
		Result struct {
			Content []map[string]any `json:"content"`
			IsError bool             `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Result.IsError || len(resp.Result.Content) != 1 || resp.Result.Content[0]["type"] != "text" {
		t.Fatalf("unexpected answer %s", w.Body.String())
	}
	if _, ok := resp.Result.Content[0]["data"]; ok {
		t.Error("text block carries a data field")
	}
	text := resp.Result.Content[0]["text"].(string)
	if !strings.Contains(text, `"id":"tagged","label":"Publisher tagged","level":"ok"`) {
		t.Errorf("collaborators did not count as tagged: %s", text)
	}
}
