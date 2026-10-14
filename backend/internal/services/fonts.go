package services

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

var (
	fontsOnce    sync.Once
	defaultFonts Fonts
	fontParseErr error
)

// Fonts are a pair of typefaces text is drawn with. The zero value means the
// built-in Go fonts.
type Fonts struct {
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
func (f Fonts) resolve() (Fonts, error) {
	if err := loadFonts(); err != nil {
		return Fonts{}, err
	}
	if f.Regular == nil {
		f.Regular = defaultFonts.Regular
	}
	if f.Bold == nil {
		f.Bold = defaultFonts.Bold
	}
	return f, nil
}

// ParseFont reads an uploaded TrueType or OpenType font, used for regular
// and bold text alike. It must be able to draw plain Latin text, umlauts
// included.
func ParseFont(data []byte) (Fonts, error) {
	f, err := opentype.Parse(data)
	if err != nil {
		return Fonts{}, fmt.Errorf("not a TrueType or OpenType font: %w", err)
	}
	var buf sfnt.Buffer
	for _, r := range "AZaz09äöüß" {
		if idx, err := f.GlyphIndex(&buf, r); err != nil || idx == 0 {
			return Fonts{}, fmt.Errorf("the font has no glyph for %q", r)
		}
	}
	return Fonts{Regular: f, Bold: f}, nil
}

func face(f *opentype.Font, size float64) font.Face {
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err) // only fails on invalid options
	}
	return fc
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
