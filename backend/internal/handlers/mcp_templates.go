package handlers

import (
	"context"
	"fmt"
	"image"
	"math"
	"strings"

	"socialmedia/internal/models"
	"socialmedia/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/image/font/opentype"
)

// compose_image can take an image editor template from the media library as
// its base: the design is drawn on the server (services/compose_template.go)
// with the changes the agent asks for, layer by layer: a new text, a
// different image (cropped to fill the layer's frame, like the editor's
// Replace), or the layer left out. list_media describes each template's
// layers by index so the agent knows what to change.

// templateLayerInfo is what list_media tells an agent about a template layer.
type templateLayerInfo struct {
	Index    int     `json:"index"`
	Kind     string  `json:"kind"`
	Text     string  `json:"text,omitempty"`
	Font     string  `json:"font,omitempty"`
	FontSize float64 `json:"fontSize,omitempty"`
	Src      string  `json:"src,omitempty"`
	// A video clip of a reel template: a still leaves it out unless an image
	// is swapped in for it.
	Clip bool `json:"clip,omitempty"`
	// shape: rect, ellipse, triangle or equalizer (a row of bars that move to
	// a reel's music), and its fill colour (empty: none).
	Shape string `json:"shape,omitempty"`
	Fill  string `json:"fill,omitempty"`
	// The layer's box (image, shape) or top-left corner (text) as fractions
	// of the canvas, ignoring rotation; paint (brush strokes) has neither.
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width,omitempty"`
	Height   float64 `json:"height,omitempty"`
	Rotation float64 `json:"rotation,omitempty"`
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// templateLayerInfos summarises a template's layers, bottom to top; nil when
// raw is not a template.
func templateLayerInfos(raw string) []templateLayerInfo {
	t, err := services.ParseTemplateDesign(raw)
	if err != nil {
		return nil
	}
	out := make([]templateLayerInfo, 0, len(t.Layers))
	for i, l := range t.Layers {
		info := templateLayerInfo{Index: i, Kind: l.Kind, Rotation: round3(l.Rotation)}
		switch l.Kind {
		case "paint":
		case "image", "shape":
			if l.Kind == "image" {
				info.Src = l.Src
				info.Clip = len(l.Clip) > 0
			} else {
				info.Shape = l.Shape
				info.Fill = l.Fill
			}
			info.X = round3((l.X - l.Width/2) / t.Width)
			info.Y = round3((l.Y - l.Height/2) / t.Height)
			info.Width = round3(l.Width / t.Width)
			info.Height = round3(l.Height / t.Height)
		default:
			info.Text = l.Text
			info.Font = l.FontFamily
			if id := services.MediaFontID(l.FontFamily); id != "" {
				info.Font = "media font " + id
			}
			info.FontSize = round3(l.FontSize / t.Width)
			info.X = round3(l.X / t.Width)
			info.Y = round3(l.Y / t.Height)
		}
		out = append(out, info)
	}
	return out
}

// loadTemplateDesign reads a template entry of the caller's media library.
func (h *MCPHandler) loadTemplateDesign(ctx context.Context, c *gin.Context, id string) (services.TemplateDesign, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return services.TemplateDesign{}, fmt.Errorf("invalid template mediaId")
	}
	filter := mcpScopeFilter(c)
	filter["_id"] = oid
	var item models.MediaItem
	if err := h.DB.Media().FindOne(ctx, filter).Decode(&item); err != nil {
		return services.TemplateDesign{}, fmt.Errorf("template %s not found", id)
	}
	if item.ContentType != models.MediaTemplateType || item.Template == "" {
		return services.TemplateDesign{}, fmt.Errorf("media %s is not a template (list_media with kind template)", id)
	}
	return services.ParseTemplateDesign(item.Template)
}

// coverCrop is the part of an iw x ih image that fills a w x h frame, the
// focus point (0..1, default centre) kept in view.
func coverCrop(iw, ih int, w, h, fx, fy float64) *services.TemplateCrop {
	if fx <= 0 || fx > 1 {
		fx = 0.5
	}
	if fy <= 0 || fy > 1 {
		fy = 0.5
	}
	aspect := w / h
	cw, ch := float64(iw), float64(ih)
	if cw/ch > aspect {
		cw = ch * aspect
	} else {
		ch = cw / aspect
	}
	return &services.TemplateCrop{X: (float64(iw) - cw) * fx, Y: (float64(ih) - ch) * fy, Width: cw, Height: ch}
}

// templateTextBlock is a text layer's size in template pixels.
func templateTextBlock(l services.TemplateLayer) (w, h float64) {
	return services.TemplateTextWidth(l), float64(len(strings.Split(l.Text, "\n"))) * l.FontSize * 1.15
}

// buildTemplate loads the template arg of compose_image, applies its layer
// changes and resolves the images and fonts the layers need.
func (h *MCPHandler) buildTemplate(ctx context.Context, c *gin.Context, arg map[string]any) (*services.TemplateDesign, error) {
	id := strArg(arg, "mediaId")
	if id == "" {
		return nil, fmt.Errorf("template: mediaId is required")
	}
	t, err := h.loadTemplateDesign(ctx, c, id)
	if err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}

	// Fonts first: a replaced text keeps its alignment point, which needs the
	// width it is drawn at.
	mediaFonts := map[string]*opentype.Font{}
	for i := range t.Layers {
		l := &t.Layers[i]
		fid := services.MediaFontID(l.FontFamily)
		if l.Kind != "text" || fid == "" {
			continue
		}
		f, ok := mediaFonts[fid]
		if !ok {
			// A font that is gone falls back to the built-in one, as the
			// editor falls back to a system font.
			f, _ = h.mediaFont(ctx, c, fid)
			mediaFonts[fid] = f
		}
		l.Font = f
	}

	load := func(ref map[string]any) (image.Image, error) { return h.loadImage(ctx, c, ref) }
	if err := applyTemplateChanges(&t, arg["layers"], load); err != nil {
		return nil, err
	}

	images := map[string]image.Image{}
	for i := range t.Layers {
		l := &t.Layers[i]
		if l.Kind == "image" && l.Image == nil {
			img, ok := images[l.Src]
			if !ok {
				var err error
				if img, err = h.loadImage(ctx, c, map[string]any{"url": l.Src}); err != nil {
					return nil, fmt.Errorf("template layer %d: %w", i, err)
				}
				images[l.Src] = img
			}
			l.Image = img
		}
	}
	return &t, nil
}

// applyTemplateChanges applies compose_image's template.layers to t: new
// texts, sizes and colours, images swapped (loaded through load) and layers
// left out. Texts must have their fonts resolved, as a replaced text keeps
// its alignment point.
func applyTemplateChanges(t *services.TemplateDesign, rawChanges any, load func(map[string]any) (image.Image, error)) error {
	changes, _ := rawChanges.([]any)
	hide := map[int]bool{}
	for n, raw := range changes {
		ch, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("template.layers[%d] must be an object", n)
		}
		iv, ok := ch["index"].(float64)
		i := int(iv)
		if !ok || iv != float64(i) || i < 0 || i >= len(t.Layers) {
			return fmt.Errorf("template.layers[%d]: index must be a layer index from 0 to %d (see templateLayers in list_media)", n, len(t.Layers)-1)
		}
		if boolArg(ch, "hide") {
			hide[i] = true
			continue
		}
		l := &t.Layers[i]
		switch l.Kind {
		case "text":
			_, hasText := ch["text"]
			scale := floatArg(ch, "fontScale")
			if hasText || scale > 0 {
				oldW, oldH := templateTextBlock(*l)
				if hasText {
					l.Text = strArg(ch, "text")
				}
				if scale > 0 {
					l.FontSize *= math.Min(scale, 5)
				}
				newW, newH := templateTextBlock(*l)
				// Keep the point the text is aligned on, and the vertical
				// centre, in the layer's rotated frame.
				var dx float64
				switch l.Align {
				case "left":
				case "right":
					dx = oldW - newW
				default:
					dx = (oldW - newW) / 2
				}
				dy := (oldH - newH) / 2
				rad := l.Rotation * math.Pi / 180
				cos, sin := math.Cos(rad), math.Sin(rad)
				l.X += dx*cos - dy*sin
				l.Y += dx*sin + dy*cos
			}
			if col := strArg(ch, "color"); col != "" {
				if _, ok := services.ParseColor(col); !ok {
					return fmt.Errorf("template.layers[%d].color: %q is not a colour", n, col)
				}
				l.Fill = col
			}
		case "shape":
			if col := strArg(ch, "color"); col != "" {
				if _, ok := services.ParseColor(col); !ok {
					return fmt.Errorf("template.layers[%d].color: %q is not a colour", n, col)
				}
				l.Fill = col
			}
		case "image":
			ref := map[string]any{}
			for _, k := range []string{"url", "mediaId", "watermarkId"} {
				if v := strArg(ch, k); v != "" {
					ref[k] = v
				}
			}
			if len(ref) == 0 {
				continue
			}
			img, err := load(ref)
			if err != nil {
				return fmt.Errorf("template.layers[%d]: %w", n, err)
			}
			l.Image = img
			l.Src = ""
			b := img.Bounds()
			l.SrcCrop = coverCrop(b.Dx(), b.Dy(), l.Width, l.Height, floatArg(ch, "focusX"), floatArg(ch, "focusY"))
		}
	}

	kept := t.Layers[:0]
	for i, l := range t.Layers {
		// A video clip cannot be drawn on a still; an image swapped in for it
		// can.
		if !hide[i] && (len(l.Clip) == 0 || l.Image != nil) {
			kept = append(kept, l)
		}
	}
	t.Layers = kept
	return nil
}

// templateCanvas is the canvas a template is drawn on: its own size, scaled
// down to the compose limit if it is larger.
func templateCanvas(t *services.TemplateDesign) (w, h int, scale float64) {
	scale = math.Min(1, float64(services.MaxComposeSide)/math.Max(t.Width, t.Height))
	return int(math.Round(t.Width * scale)), int(math.Round(t.Height * scale)), scale
}
