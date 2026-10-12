package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Radar slides are 4:5 portrait, the tallest format a feed carousel shows
// uncropped. Every slide of a carousel shares the first slide's ratio, so the
// cover and closing images are fitted to it too.
const (
	RadarSlideWidth  = 1080
	RadarSlideHeight = 1350
)

var (
	radarBg      = color.RGBA{0x0f, 0x17, 0x2a, 0xff}
	radarPanel   = color.RGBA{0x1e, 0x29, 0x3b, 0xff}
	radarText    = color.RGBA{0xf8, 0xfa, 0xfc, 0xff}
	radarMuted   = color.RGBA{0x94, 0xa3, 0xb8, 0xff}
	badgeYes     = color.RGBA{0x16, 0xa3, 0x4a, 0xff}
	badgeNo      = color.RGBA{0xdc, 0x26, 0x26, 0xff}
	badgeOpen    = color.RGBA{0xd9, 0x77, 0x06, 0xff}
	fontsOnce    sync.Once
	defaultFonts RadarFonts
	fontParseErr error
)

// RadarFonts are the typefaces the slides are drawn with: Regular for the
// subtitle and badge question, Bold for everything else. The zero value
// means the built-in Go fonts.
type RadarFonts struct {
	Regular *opentype.Font
	Bold    *opentype.Font
}

func loadFonts() error {
	fontsOnce.Do(func() {
		defaultFonts.Regular, fontParseErr = opentype.Parse(goregular.TTF)
		if fontParseErr != nil {
			return
		}
		defaultFonts.Bold, fontParseErr = opentype.Parse(gobold.TTF)
	})
	return fontParseErr
}

// resolve fills in the built-in fonts for any face not set.
func (f RadarFonts) resolve() (RadarFonts, error) {
	if err := loadFonts(); err != nil {
		return RadarFonts{}, err
	}
	if f.Regular == nil {
		f.Regular = defaultFonts.Regular
	}
	if f.Bold == nil {
		f.Bold = defaultFonts.Bold
	}
	return f, nil
}

// ParseRadarFont reads an uploaded TrueType or OpenType font. A team uploads
// one file, which is used for every text on the slides, so it must be able
// to draw plain Latin text, umlauts included.
func ParseRadarFont(data []byte) (RadarFonts, error) {
	f, err := opentype.Parse(data)
	if err != nil {
		return RadarFonts{}, fmt.Errorf("not a TrueType or OpenType font: %w", err)
	}
	var buf sfnt.Buffer
	for _, r := range "AZaz09äöüß" {
		if idx, err := f.GlyphIndex(&buf, r); err != nil || idx == 0 {
			return RadarFonts{}, fmt.Errorf("the font has no glyph for %q", r)
		}
	}
	return RadarFonts{Regular: f, Bold: f}, nil
}

func face(f *opentype.Font, size float64) font.Face {
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err) // only fails on invalid options
	}
	return fc
}

// ParseHexColor reads "#rrggbb", falling back to the default accent.
func ParseHexColor(s string) color.RGBA {
	c := color.RGBA{0x63, 0x66, 0xf1, 0xff}
	if len(s) == 7 && s[0] == '#' {
		var r, g, b uint8
		if _, err := fmt.Sscanf(s[1:], "%02x%02x%02x", &r, &g, &b); err == nil {
			c = color.RGBA{r, g, b, 0xff}
		}
	}
	return c
}

func newSlide(bg color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, RadarSlideWidth, RadarSlideHeight))
	xdraw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, xdraw.Src)
	return img
}

func encodeSlide(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fillRoundedRect paints r with corners of the given radius.
func fillRoundedRect(dst *image.RGBA, r image.Rectangle, radius int, c color.Color) {
	src := &image.Uniform{c}
	mask := image.NewAlpha(r)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cx, cy := x, y
			switch {
			case x < r.Min.X+radius:
				cx = r.Min.X + radius
			case x >= r.Max.X-radius:
				cx = r.Max.X - radius - 1
			}
			switch {
			case y < r.Min.Y+radius:
				cy = r.Min.Y + radius
			case y >= r.Max.Y-radius:
				cy = r.Max.Y - radius - 1
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= radius*radius {
				mask.SetAlpha(x, y, color.Alpha{0xff})
			}
		}
	}
	xdraw.DrawMask(dst, r, src, image.Point{}, mask, r.Min, xdraw.Over)
}

// fitInto scales src to fit entirely inside box (letterboxed), centred.
func fitInto(dst *image.RGBA, box image.Rectangle, src image.Image) {
	sb := src.Bounds()
	if sb.Dx() == 0 || sb.Dy() == 0 {
		return
	}
	scale := min(float64(box.Dx())/float64(sb.Dx()), float64(box.Dy())/float64(sb.Dy()))
	w, h := int(float64(sb.Dx())*scale), int(float64(sb.Dy())*scale)
	x := box.Min.X + (box.Dx()-w)/2
	y := box.Min.Y + (box.Dy()-h)/2
	xdraw.CatmullRom.Scale(dst, image.Rect(x, y, x+w, y+h), src, sb, xdraw.Over, nil)
}

// fillCover scales src to cover the whole slide, cropping the overflow
// evenly on both sides.
func fillCover(dst *image.RGBA, src image.Image) {
	sb := src.Bounds()
	if sb.Dx() == 0 || sb.Dy() == 0 {
		return
	}
	db := dst.Bounds()
	scale := max(float64(db.Dx())/float64(sb.Dx()), float64(db.Dy())/float64(sb.Dy()))
	cropW, cropH := int(float64(db.Dx())/scale), int(float64(db.Dy())/scale)
	x0 := sb.Min.X + (sb.Dx()-cropW)/2
	y0 := sb.Min.Y + (sb.Dy()-cropH)/2
	xdraw.CatmullRom.Scale(dst, db, src, image.Rect(x0, y0, x0+cropW, y0+cropH), xdraw.Src, nil)
}

func textWidth(f font.Face, s string) int {
	return font.MeasureString(f, s).Round()
}

// wrapText breaks s into at most maxLines lines no wider than width,
// ellipsizing the last one when the text does not fit.
func wrapText(f font.Face, s string, width, maxLines int) []string {
	words := strings.Fields(s)
	var lines []string
	cur := ""
	for i := 0; i < len(words); i++ {
		w := words[i]
		try := w
		if cur != "" {
			try = cur + " " + w
		}
		if textWidth(f, try) <= width || cur == "" {
			cur = try
			continue
		}
		lines = append(lines, cur)
		cur = w
		if len(lines) == maxLines {
			cur = ""
			// Words are left over: mark the cut on the last line.
			last := lines[maxLines-1]
			for textWidth(f, last+"…") > width && len(last) > 0 {
				r := []rune(last)
				last = string(r[:len(r)-1])
			}
			lines[maxLines-1] = strings.TrimSpace(last) + "…"
			return lines
		}
	}
	if cur != "" {
		// A single word wider than the line still has to be cut.
		for textWidth(f, cur) > width && len([]rune(cur)) > 1 {
			r := []rune(cur)
			cur = string(r[:len(r)-2]) + "…"
		}
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return lines
}

func drawText(dst *image.RGBA, f font.Face, c color.Color, x, baseline int, s string) {
	d := &font.Drawer{Dst: dst, Src: &image.Uniform{c}, Face: f, Dot: fixed.P(x, baseline)}
	d.DrawString(s)
}

func drawCentered(dst *image.RGBA, f font.Face, c color.Color, baseline int, s string) {
	drawText(dst, f, c, (RadarSlideWidth-textWidth(f, s))/2, baseline, s)
}

// RenderRadarCover draws the opening slide. With a cover image the image is
// the slide; without one the title and subtitle are drawn on the accent
// colour.
func RenderRadarCover(coverImg image.Image, title, subtitle string, accent color.RGBA, fonts RadarFonts) ([]byte, error) {
	fonts, err := fonts.resolve()
	if err != nil {
		return nil, err
	}
	slide := newSlide(accent)
	if coverImg != nil {
		fillCover(slide, coverImg)
		return encodeSlide(slide)
	}
	titleFace := face(fonts.Bold, 110)
	lines := wrapText(titleFace, title, RadarSlideWidth-160, 3)
	lineH := 124
	y := RadarSlideHeight/2 - (len(lines)*lineH)/2 + 90
	for _, l := range lines {
		drawCentered(slide, titleFace, radarText, y, l)
		y += lineH
	}
	if subtitle != "" {
		drawCentered(slide, face(fonts.Regular, 46), radarText, y+30, subtitle)
	}
	return encodeSlide(slide)
}

// RadarItemSlide is one game of the Radar.
type RadarItemSlide struct {
	Index         int
	Total         int
	Title         string
	Image         image.Image
	GermanRelease string // "ja", "nein", "offen" or "" for no badge
}

// RenderRadarItem draws one game slide: header, the game image, its title and
// the "Kommt's auf Deutsch?" badge.
func RenderRadarItem(item RadarItemSlide, header, badgeQuestion string, accent color.RGBA, fonts RadarFonts) ([]byte, error) {
	fonts, err := fonts.resolve()
	if err != nil {
		return nil, err
	}
	slide := newSlide(radarBg)

	// Header: the Radar title on the left, the position on the right.
	headerFace := face(fonts.Bold, 40)
	drawText(slide, headerFace, accent, 60, 92, strings.ToUpper(header))
	pos := fmt.Sprintf("%d/%d", item.Index, item.Total)
	drawText(slide, headerFace, radarMuted, RadarSlideWidth-60-textWidth(headerFace, pos), 92, pos)

	// The game image on a panel.
	box := image.Rect(60, 130, RadarSlideWidth-60, 130+880)
	fillRoundedRect(slide, box, 28, radarPanel)
	if item.Image != nil {
		fitInto(slide, box.Inset(20), item.Image)
	}

	// Title, up to two lines.
	titleFace := face(fonts.Bold, 62)
	y := 1100
	for _, l := range wrapText(titleFace, item.Title, RadarSlideWidth-120, 2) {
		drawText(slide, titleFace, radarText, 60, y, l)
		y += 72
	}

	// Badge: the question on the left, the answer as a coloured pill.
	if item.GermanRelease != "" {
		label, bg := "OFFEN", badgeOpen
		switch item.GermanRelease {
		case "ja":
			label, bg = "JA", badgeYes
		case "nein":
			label, bg = "NEIN", badgeNo
		}
		pillFace := face(fonts.Bold, 52)
		pillW := textWidth(pillFace, label) + 80
		pill := image.Rect(RadarSlideWidth-60-pillW, 1222, RadarSlideWidth-60, 1222+84)
		fillRoundedRect(slide, pill, 42, bg)
		drawText(slide, pillFace, radarText, pill.Min.X+40, pill.Min.Y+61, label)

		qFace := face(fonts.Regular, 44)
		q := wrapText(qFace, badgeQuestion, pill.Min.X-60-30, 1)
		if len(q) > 0 {
			drawText(slide, qFace, radarMuted, 60, pill.Min.Y+57, q[0])
		}
	}
	return encodeSlide(slide)
}

// RenderRadarClosing draws the last slide: the closing image when there is
// one, otherwise the closing text on the dark background.
func RenderRadarClosing(closingImg image.Image, text string, accent color.RGBA, fonts RadarFonts) ([]byte, error) {
	fonts, err := fonts.resolve()
	if err != nil {
		return nil, err
	}
	slide := newSlide(radarBg)
	if closingImg != nil {
		fillCover(slide, closingImg)
		return encodeSlide(slide)
	}
	fillRoundedRect(slide, image.Rect(60, 60, RadarSlideWidth-60, 72), 6, accent)
	textFace := face(fonts.Bold, 76)
	lines := wrapText(textFace, text, RadarSlideWidth-160, 6)
	lineH := 92
	y := RadarSlideHeight/2 - (len(lines)*lineH)/2 + 60
	for _, l := range lines {
		drawCentered(slide, textFace, radarText, y, l)
		y += lineH
	}
	return encodeSlide(slide)
}
