package services

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"socialmedia/internal/models"

	"golang.org/x/image/font/gofont/gomono"
)

func TestNormalizeIGUsernames(t *testing.T) {
	got := NormalizeIGUsernames([]string{"@Feuerland.Spiele", "feuerland.spiele", " pegasusspiele ", "not valid!", "@tabulaludo.podcast", "", "a", "b"}, "tabulaludo.podcast", 3)
	if strings.Join(got, ",") != "Feuerland.Spiele,pegasusspiele,a" {
		t.Errorf("normalized = %v", got)
	}
}

func TestUserTagsParam(t *testing.T) {
	var tags []map[string]any
	if err := json.Unmarshal([]byte(userTagsParam([]string{"a", "b"}, true)), &tags); err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0]["username"] != "a" || tags[0]["x"] == nil || tags[1]["y"] == nil {
		t.Errorf("image tags = %v", tags)
	}
	tags = nil
	if err := json.Unmarshal([]byte(userTagsParam([]string{"a"}, false)), &tags); err != nil {
		t.Fatal(err)
	}
	if _, hasX := tags[0]["x"]; hasX {
		t.Errorf("reel tags must not carry a position: %v", tags)
	}
}

func TestInsightsResponseValues(t *testing.T) {
	var r insightsResponse
	body := `{"data":[{"name":"reach","values":[{"value":120}]},{"name":"views","total_value":{"value":340}}]}`
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	v := r.values()
	if v["reach"] != 120 || v["views"] != 340 {
		t.Errorf("values = %v", v)
	}
}

func TestInsightsDue(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-5 * time.Hour)
	if !insightsDue(fresh, nil, now) {
		t.Error("a post without numbers is always due")
	}
	if insightsDue(fresh, &models.PostInsights{FetchedAt: now.Add(-time.Hour)}, now) {
		t.Error("a fresh post read an hour ago is not due yet")
	}
	if !insightsDue(fresh, &models.PostInsights{FetchedAt: now.Add(-4 * time.Hour)}, now) {
		t.Error("a fresh post read four hours ago is due")
	}
	old := now.AddDate(0, 0, -20)
	if insightsDue(old, &models.PostInsights{FetchedAt: now.Add(-24 * time.Hour)}, now) {
		t.Error("an old post read a day ago is not due yet")
	}
}

func decodeSlide(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("slide is not a JPEG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != RadarSlideWidth || b.Dy() != RadarSlideHeight {
		t.Fatalf("slide is %dx%d, want %dx%d", b.Dx(), b.Dy(), RadarSlideWidth, RadarSlideHeight)
	}
	return img
}

func TestRenderRadarSlides(t *testing.T) {
	colors := DefaultRadarColors()
	cover, err := RenderRadarCover(nil, "Radar der Woche", "KW 42 · 5 Ankündigungen", colors, RadarFonts{})
	if err != nil {
		t.Fatal(err)
	}
	decodeSlide(t, cover)

	game := image.NewRGBA(image.Rect(0, 0, 600, 800))
	for i := range game.Pix {
		game.Pix[i] = 200
	}
	item, err := RenderRadarItem(RadarItemSlide{
		Index: 1, Total: 5, Title: "Ein ziemlich langer Spieltitel, der sicher umbrechen muss, weil er so lang ist", Image: game, GermanRelease: "ja",
	}, "Radar der Woche", "Kommt's auf Deutsch?", colors, RadarFonts{})
	if err != nil {
		t.Fatal(err)
	}
	img := decodeSlide(t, item)
	// The game image is drawn inside the panel.
	r, g, b, _ := img.At(RadarSlideWidth/2, 570).RGBA()
	if r>>8 < 150 || g>>8 < 150 || b>>8 < 150 {
		t.Errorf("game image missing from its panel, pixel = %v", color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255})
	}

	closing, err := RenderRadarClosing(nil, "Welche Neuheit interessiert euch am meisten?", colors, RadarFonts{})
	if err != nil {
		t.Fatal(err)
	}
	decodeSlide(t, closing)

	// A cover image fills the whole slide.
	coverImg := image.NewRGBA(image.Rect(0, 0, 1080, 1080))
	for i := range coverImg.Pix {
		coverImg.Pix[i] = 255
	}
	withImage, err := RenderRadarCover(coverImg, "ignored", "", colors, RadarFonts{})
	if err != nil {
		t.Fatal(err)
	}
	img = decodeSlide(t, withImage)
	if r, _, _, _ := img.At(5, 5).RGBA(); r>>8 < 240 {
		t.Errorf("cover image does not reach the corner")
	}
}

func TestRadarColors(t *testing.T) {
	c := ParseRadarColors(RadarColorHexes{Background: "#102030", BadgeNo: "#abcdef", Panel: "nonsense"})
	if c.Background != (color.RGBA{0x10, 0x20, 0x30, 0xff}) || c.BadgeNo != (color.RGBA{0xab, 0xcd, 0xef, 0xff}) {
		t.Errorf("set colours not used: %+v", c)
	}
	def := DefaultRadarColors()
	if c.Panel != def.Panel || c.Accent != def.Accent {
		t.Errorf("empty or invalid colours should take the default: %+v", c)
	}

	// The item slide is drawn in the team's colours.
	c.Background = color.RGBA{0x20, 0xc0, 0x40, 0xff}
	slide, err := RenderRadarItem(RadarItemSlide{Index: 1, Total: 1, Title: "Spiel", GermanRelease: "nein"}, "Radar", "Kommt's?", c, RadarFonts{})
	if err != nil {
		t.Fatal(err)
	}
	img := decodeSlide(t, slide)
	near := func(x, y int, want color.RGBA) {
		t.Helper()
		r, g, b, _ := img.At(x, y).RGBA()
		d := func(a uint32, b uint8) int {
			v := int(a>>8) - int(b)
			if v < 0 {
				v = -v
			}
			return v
		}
		if d(r, want.R) > 12 || d(g, want.G) > 12 || d(b, want.B) > 12 {
			t.Errorf("pixel (%d,%d) = %d,%d,%d, want about %v", x, y, r>>8, g>>8, b>>8, want)
		}
	}
	near(10, 10, c.Background)
	near(RadarSlideWidth/2, 570, c.Panel)
	near(RadarSlideWidth-70, 1264, c.BadgeNo)
}

func TestWrapText(t *testing.T) {
	if err := loadFonts(); err != nil {
		t.Fatal(err)
	}
	f := face(defaultFonts.Bold, 62)
	lines := wrapText(f, "eins zwei drei vier fünf sechs sieben acht neun zehn elf zwölf dreizehn vierzehn", 400, 2)
	if len(lines) != 2 || !strings.HasSuffix(lines[1], "…") {
		t.Errorf("lines = %q, want two with an ellipsis", lines)
	}
	for _, l := range lines {
		if textWidth(f, l) > 400 {
			t.Errorf("line %q is wider than 400px", l)
		}
	}
}

func TestRadarCustomFont(t *testing.T) {
	fonts, err := ParseRadarFont(gomono.TTF)
	if err != nil {
		t.Fatalf("a valid font was rejected: %v", err)
	}
	if fonts.Regular == nil || fonts.Bold == nil {
		t.Fatal("the uploaded font should serve both faces")
	}
	if _, err := ParseRadarFont([]byte("not a font")); err == nil {
		t.Error("garbage was accepted as a font")
	}

	colors := DefaultRadarColors()
	custom, err := RenderRadarCover(nil, "Radar der Woche", "KW 42", colors, fonts)
	if err != nil {
		t.Fatal(err)
	}
	decodeSlide(t, custom)
	builtin, err := RenderRadarCover(nil, "Radar der Woche", "KW 42", colors, RadarFonts{})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(custom, builtin) {
		t.Error("the custom font made no difference to the slide")
	}
}
