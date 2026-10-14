package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"socialmedia/internal/models"
	"socialmedia/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/image/font/opentype"
)

// Image tools for agents: watermarks with descriptions and roles, the media
// library, server-side composition and previews the agent can look at.

// mcpRichResult is a tool result that carries images next to its JSON: each
// becomes an MCP image content block after the text block, so a multimodal
// client can see what it built.
type mcpRichResult struct {
	Result any
	Images []mcpImage
}

type mcpImage struct {
	Data     []byte
	MimeType string
}

// previewOf encodes a thumbnail of img for an image content block.
func previewOf(img image.Image, maxSide int) []mcpImage {
	data, _, _, err := services.Thumbnail(img, maxSide)
	if err != nil {
		return nil
	}
	return []mcpImage{{Data: data, MimeType: "image/jpeg"}}
}

func imageToolDefinitions() []mcpTool {
	layerSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type":         map[string]any{"type": "string", "enum": []string{"image", "text", "rect", "gradient"}, "description": "Layer kind"},
			"watermarkId":  map[string]any{"type": "string", "description": "image: a team watermark (see list_watermarks)"},
			"mediaId":      map[string]any{"type": "string", "description": "image: a media library entry (see list_media)"},
			"url":          map[string]any{"type": "string", "description": "image: an upload URL (/api/uploads/...) or a public image URL"},
			"anchor":       map[string]any{"type": "string", "enum": []string{"top-left", "top", "top-right", "left", "center", "right", "bottom-left", "bottom", "bottom-right", "full"}, "description": "Where the layer sits (default center). full stretches an image or rect over the whole canvas, which is how frame overlays and most team watermarks are meant to be used. For a gradient: the edge it darkens towards (top, bottom, left, right; default bottom)."},
			"margin":       map[string]any{"type": "number", "description": "Distance from the anchored edges, fraction of the canvas width (e.g. 0.04)"},
			"x":            map[string]any{"type": "number", "description": "Left edge as a fraction of the canvas width; with y, overrides anchor"},
			"y":            map[string]any{"type": "number", "description": "Top edge as a fraction of the canvas height"},
			"width":        map[string]any{"type": "number", "description": "Width as a fraction of the canvas width (image: aspect kept when height is omitted; text: wrap width; gradient left/right: extent)"},
			"height":       map[string]any{"type": "number", "description": "Height as a fraction of the canvas height (gradient top/bottom: extent, default 0.45)"},
			"opacity":      map[string]any{"type": "number", "description": "0 to 1 (default 1)"},
			"text":         map[string]any{"type": "string", "description": "text: the text; \\n starts a new line, long lines wrap"},
			"font":         map[string]any{"type": "string", "enum": []string{"bold", "regular"}, "description": "text: built-in bold (default) or regular"},
			"fontMediaId":  map[string]any{"type": "string", "description": "text: a font from the media library (list_media with kind font); overrides font"},
			"fontSize":     map[string]any{"type": "number", "description": "text: font size as a fraction of the canvas width (default 0.06; headlines 0.07-0.1, captions 0.035-0.05)"},
			"color":        map[string]any{"type": "string", "description": "text/rect/gradient colour, #rgb, #rrggbb or #rrggbbaa (text default white, gradient default black)"},
			"align":        map[string]any{"type": "string", "enum": []string{"left", "center", "right"}, "description": "text alignment (default follows the anchor)"},
			"maxLines":     map[string]any{"type": "number", "description": "text: maximum lines, the rest is cut with … (default 6)"},
			"outlineColor": map[string]any{"type": "string", "description": "text outline colour"},
			"outlineWidth": map[string]any{"type": "number", "description": "text outline width as a fraction of the font size (e.g. 0.06)"},
			"shadow":       map[string]any{"type": "boolean", "description": "text drop shadow"},
			"boxColor":     map[string]any{"type": "string", "description": "text: background box colour, e.g. #000000b3"},
			"boxPadding":   map[string]any{"type": "number", "description": "text: box padding as a fraction of the font size (default 0.4)"},
			"radius":       map[string]any{"type": "number", "description": "rect/text box corner radius, fraction of the canvas width"},
		},
		"required": []string{"type"},
	}
	return []mcpTool{
		{
			Name: "update_watermark",
			Description: "Rename a watermark or set its description. Describe what it looks like and what it is for " +
				"(e.g. \"round logo, place in a corner at 15-20% width\" or \"full-size frame with the news banner at the bottom, use with anchor full\"), so compose_image can pick it.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string", "description": "Watermark ID"},
					"name":        map[string]any{"type": "string", "description": "New name"},
					"description": map[string]any{"type": "string", "description": "New description"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name: "list_media",
			Description: "Search the team's media library: uploaded photos, artwork, logos, backgrounds and fonts, and the team's colours, each with a name, a description and tags. " +
				"All words of query must appear in the name, description, tags or colour value; every tag in tags must be set; kind limits the results to images, videos, fonts, colours or templates. " +
				"A font's id goes into fontMediaId of a compose_image text layer. A colour entry has no file: its color (#rrggbb or #rrggbbaa) goes into any colour field of compose_image, " +
				"and its description says how and where the team uses it; follow that. " +
				"A template is a design of the team's image editor: its templateLayers list each layer by index (texts with their wording, images with their frame), " +
				"and compose_image with template draws it with the texts and images you swap in. " +
				"With preview=true a small thumbnail of each hit (up to 12) is attached; for a font, a text sample set in it; for a template, its preview.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":   map[string]any{"type": "string", "description": "Words to search for"},
					"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Required tags"},
					"kind":    map[string]any{"type": "string", "enum": []string{"image", "video", "font", "color", "template"}, "description": "Only this kind of entry (default all). A template is a design for the image editor; its url is a preview image"},
					"limit":   map[string]any{"type": "number", "description": "Maximum results (default 50, max 200)"},
					"preview": map[string]any{"type": "boolean", "description": "Attach thumbnails"},
				},
			},
		},
		{
			Name: "add_media",
			Description: "Add a file to the media library from a URL (an upload URL or a public URL) or from base64 data, with a name, a description of what it shows and tags. " +
				"Images, videos (mp4, mov) and fonts (.ttf, .otf; a font must draw Latin text with umlauts, max 10 MB) are accepted. " +
				"Or add a colour: give color instead of url/data, and say in description how to use it.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":         map[string]any{"type": "string", "description": "Upload URL (/api/uploads/...) or public http(s) URL"},
					"data":        map[string]any{"type": "string", "description": "Base64 file data, instead of url"},
					"filename":    map[string]any{"type": "string", "description": "File name with extension, required with data"},
					"color":       map[string]any{"type": "string", "description": "A colour (#rgb, #rrggbb or #rrggbbaa), instead of url or data"},
					"name":        map[string]any{"type": "string", "description": "Display name"},
					"description": map[string]any{"type": "string", "description": "What the file shows, or for a colour how to use it"},
					"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags"},
				},
			},
		},
		{
			Name:        "update_media",
			Description: "Change a media library entry's name, description or tags, or a colour entry's color. Only provided fields are changed; tags replaces the whole list.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string", "description": "Media ID"},
					"name":        map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"color":       map[string]any{"type": "string", "description": "Colour entries only: the new colour"},
					"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "delete_media",
			Description: "Remove an entry (a file or a colour) from the media library. A file is freed unless a post, draft or anything else still uses it.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Media ID"}},
				"required":   []string{"id"},
			},
		},
		{
			Name: "get_image",
			Description: "Look at an image: returns its size and attaches a preview. Use it to check a watermark, a media entry, a found image or a post image before using it. " +
				"Give url, watermarkId or mediaId. For a font in the media library the preview is a text sample set in it.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":         map[string]any{"type": "string", "description": "Upload URL or public image URL"},
					"watermarkId": map[string]any{"type": "string"},
					"mediaId":     map[string]any{"type": "string"},
					"maxSide":     map[string]any{"type": "number", "description": "Longest side of the preview in px (default 768, max 1536)"},
				},
			},
		},
		{
			Name: "compose_image",
			Description: "Build a post image on the server: a base image (or a plain background) fitted to a format, an optional filter, and layers on top: " +
				"team watermarks and media library images, text (headline, caption, badge), rectangles and gradients. " +
				"Or start from a template of the media library (template: the team's own designs, with texts and images swapped per layer), which keeps a series of images in the team's design. " +
				"The result is stored as an upload; " +
				"pass its url in imageUrls of create_post or update_post. A preview is attached so you can check it and adjust. " +
				"Team watermarks are often full-canvas PNG overlays (frames, banners): put those on top with anchor full. " +
				"Take colours from the team's palette (list_media with kind color) and use each as its description says. " +
				"For text over a photo, add a gradient or a boxColor so it stays legible. Keep text short; Instagram images carry a few words, not the caption.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"template": map[string]any{
						"type": "object",
						"description": "An image editor template from the media library (list_media with kind template, which lists each template's layers as templateLayers) drawn as the design: " +
							"its canvas size and background, then its layers with the changes in layers. format, width and height are ignored; a base, if given, goes under the template's layers, and layers on top of them. " +
							"Use it for series like carousel slides: the same template every slide, only the texts and images changed.",
						"properties": map[string]any{
							"mediaId": map[string]any{"type": "string", "description": "The template's media id"},
							"layers": map[string]any{
								"type":        "array",
								"description": "Changes to the template's layers, by index (templateLayers[].index). Layers not named stay as designed.",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"index":       map[string]any{"type": "number", "description": "The layer to change"},
										"hide":        map[string]any{"type": "boolean", "description": "Leave the layer out"},
										"text":        map[string]any{"type": "string", "description": "text layer: the new text (\\n starts a new line; lines do not wrap). It keeps the layer's font, size, colour and alignment point"},
										"fontScale":   map[string]any{"type": "number", "description": "text layer: multiply the font size, e.g. 0.8 for a longer text"},
										"color":       map[string]any{"type": "string", "description": "text layer: new text colour"},
										"url":         map[string]any{"type": "string", "description": "image layer: replace the image (upload or public URL); it is cropped to fill the layer's frame, corners and border kept"},
										"mediaId":     map[string]any{"type": "string", "description": "image layer: replace the image with a media library image"},
										"watermarkId": map[string]any{"type": "string", "description": "image layer: replace the image with a team watermark"},
										"focusX":      map[string]any{"type": "number", "description": "image replacement: horizontal point to keep in the frame, 0 left to 1 right (default 0.5)"},
										"focusY":      map[string]any{"type": "number", "description": "image replacement: vertical point to keep in the frame, 0 top to 1 bottom (default 0.5)"},
									},
									"required": []string{"index"},
								},
							},
						},
						"required": []string{"mediaId"},
					},
					"base": map[string]any{
						"type":        "object",
						"description": "Base image: one of url, mediaId or watermarkId. Omit for a plain background.",
						"properties": map[string]any{
							"url":         map[string]any{"type": "string"},
							"mediaId":     map[string]any{"type": "string"},
							"watermarkId": map[string]any{"type": "string"},
						},
					},
					"format":     map[string]any{"type": "string", "enum": []string{"1:1", "4:5", "9:16", "16:9", "1.91:1", "original"}, "description": "Canvas format (default 4:5, the tallest a feed post shows uncropped; 9:16 for stories and reels; original keeps the base image's ratio)"},
					"width":      map[string]any{"type": "number", "description": "Canvas width in px (default 1080); with height, overrides format"},
					"height":     map[string]any{"type": "number", "description": "Canvas height in px"},
					"fit":        map[string]any{"type": "string", "enum": []string{"cover", "contain", "blur"}, "description": "How the base fills the canvas: cover crops (default), contain letterboxes on the background colour, blur letterboxes on a blurred copy (good for box art and covers)"},
					"focusX":     map[string]any{"type": "number", "description": "cover: horizontal point to keep in view, 0 left to 1 right (default 0.5)"},
					"focusY":     map[string]any{"type": "number", "description": "cover: vertical point to keep in view, 0 top to 1 bottom (default 0.5)"},
					"background": map[string]any{"type": "string", "description": "Background colour (default #000000)"},
					"filter":     map[string]any{"type": "string", "enum": []string{"none", "grayscale", "sepia", "warm", "cool", "vivid", "fade", "dramatic"}, "description": "Filter preset for the base"},
					"brightness": map[string]any{"type": "number", "description": "-1 to 1"},
					"contrast":   map[string]any{"type": "number", "description": "-1 to 1"},
					"saturation": map[string]any{"type": "number", "description": "-1 to 1"},
					"layers":     map[string]any{"type": "array", "items": layerSchema, "description": "Layers, bottom to top"},
					"addToMedia": map[string]any{"type": "boolean", "description": "Also add the result to the media library"},
					"name":       map[string]any{"type": "string", "description": "Media library name when addToMedia is set"},
					"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Media library tags when addToMedia is set"},
				},
			},
		},
	}
}

// --- image loading ---

const maxSourceImageBytes = 25 * 1024 * 1024

// loadImageBytes reads an image the agent referred to by upload URL, public
// URL, watermark or media entry. Remote URLs go through the SSRF-safe client.
func (h *MCPHandler) loadImageBytes(ctx context.Context, c *gin.Context, ref map[string]any) ([]byte, string, error) {
	if id := strArg(ref, "watermarkId"); id != "" {
		oid, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return nil, "", fmt.Errorf("invalid watermarkId")
		}
		filter := mcpScopeFilter(c)
		filter["_id"] = oid
		var wm models.Watermark
		if err := h.DB.Watermarks().FindOne(ctx, filter).Decode(&wm); err != nil {
			return nil, "", fmt.Errorf("watermark %s not found", id)
		}
		data, err := readImageBytes(ctx, h.DB, h.UploadDir, wm.URL)
		return data, wm.URL, err
	}
	if id := strArg(ref, "mediaId"); id != "" {
		oid, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return nil, "", fmt.Errorf("invalid mediaId")
		}
		filter := mcpScopeFilter(c)
		filter["_id"] = oid
		var item models.MediaItem
		if err := h.DB.Media().FindOne(ctx, filter).Decode(&item); err != nil {
			return nil, "", fmt.Errorf("media %s not found", id)
		}
		if item.Color != "" {
			return nil, "", fmt.Errorf("media %s is the colour %s, not a file: use it in a colour field", id, item.Color)
		}
		data, err := readImageBytes(ctx, h.DB, h.UploadDir, item.URL)
		return data, item.URL, err
	}
	raw := strings.TrimSpace(strArg(ref, "url"))
	if raw == "" {
		return nil, "", fmt.Errorf("give url, mediaId or watermarkId")
	}
	// An absolute link to one of our own uploads counts as the upload.
	if i := strings.Index(raw, uploadURLPrefix); i > 0 && (strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")) {
		if appBaseURL != "" && strings.HasPrefix(raw, strings.TrimRight(appBaseURL, "/")+uploadURLPrefix) {
			raw = raw[i:]
		}
	}
	if uploadFilename(raw) != "" {
		data, err := readImageBytes(ctx, h.DB, h.UploadDir, raw)
		return data, raw, err
	}
	data, err := fetchRemoteImage(ctx, raw)
	return data, raw, err
}

func fetchRemoteImage(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if err := validateURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SocialPod/1.0)")
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	resp, err := safeURLClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxSourceImageBytes))
}

func decodeImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("not a supported image (jpeg, png, gif, webp): %w", err)
	}
	if o := services.ReadExifOrientation(data); o >= 2 && o <= 8 {
		img = services.ApplyOrientation(img, o)
	}
	return img, nil
}

func (h *MCPHandler) loadImage(ctx context.Context, c *gin.Context, ref map[string]any) (image.Image, error) {
	data, _, err := h.loadImageBytes(ctx, c, ref)
	if err != nil {
		return nil, err
	}
	return decodeImage(data)
}

// --- watermarks ---

// watermarkRoles maps watermark IDs to what the team uses them for.
func (h *MCPHandler) watermarkRoles(ctx context.Context, c *gin.Context) map[primitive.ObjectID][]string {
	roles := map[primitive.ObjectID][]string{}
	teamID := teamIDFrom(c)
	if teamID == nil {
		return roles
	}
	var team models.Team
	if err := h.DB.Teams().FindOne(ctx, bson.M{"_id": teamID}).Decode(&team); err != nil {
		return roles
	}
	add := func(id *primitive.ObjectID, role string) {
		if id != nil {
			roles[*id] = append(roles[*id], role)
		}
	}
	add(team.BGGWatermarkID, "BGG cover overlay (import_bgg default)")
	add(team.NewsCreatorWatermarkID, "news article image overlay")
	add(team.EpisodeCreatorWatermarkID, "episode image overlay")
	add(team.EpisodeOverlayNewsID, "news episode cover overlay")
	add(team.EpisodeOverlayReviewID, "review episode cover overlay")
	add(team.EpisodeOverlaySpecialID, "special episode cover overlay")
	add(team.EpisodeOverlayTatortID, "Tatort episode cover overlay")
	return roles
}

type watermarkInfo struct {
	models.Watermark
	AbsoluteURL string   `json:"absoluteUrl"`
	Roles       []string `json:"roles,omitempty"`
	Width       int      `json:"width,omitempty"`
	Height      int      `json:"height,omitempty"`
	// FullCanvas marks an overlay meant to cover the whole image (a frame or
	// banner with a transparent middle) rather than a small logo.
	FullCanvas bool `json:"fullCanvas"`
}

func (h *MCPHandler) toolListWatermarks(c *gin.Context, args map[string]any) (any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cursor, err := h.DB.Watermarks().Find(ctx, mcpScopeFilter(c))
	if err != nil {
		return map[string]string{"error": "Failed to fetch watermarks"}, true
	}
	var watermarks []models.Watermark
	if err := cursor.All(ctx, &watermarks); err != nil {
		return map[string]string{"error": "Failed to decode watermarks"}, true
	}
	roles := h.watermarkRoles(ctx, c)
	preview := boolArg(args, "preview")
	out := []watermarkInfo{}
	var images []mcpImage
	for _, wm := range watermarks {
		info := watermarkInfo{Watermark: wm, AbsoluteURL: absoluteURL(c, wm.URL), Roles: roles[wm.ID]}
		if data, err := readImageBytes(ctx, h.DB, h.UploadDir, wm.URL); err == nil {
			if img, err := decodeImage(data); err == nil {
				b := img.Bounds()
				info.Width, info.Height = b.Dx(), b.Dy()
				info.FullCanvas = isFullCanvasOverlay(img)
				if preview && len(images) < 12 {
					images = append(images, previewOf(img, 256)...)
				}
			}
		}
		out = append(out, info)
	}
	if len(images) > 0 {
		return &mcpRichResult{Result: map[string]any{"watermarks": out, "previews": "attached in list order, on grey"}, Images: images}, false
	}
	return out, false
}

// isFullCanvasOverlay reports whether a watermark is a large overlay with a
// transparent middle (a frame or banner), as opposed to a small opaque logo:
// big enough to be a post image, with a mostly transparent centre.
func isFullCanvasOverlay(img image.Image) bool {
	b := img.Bounds()
	if b.Dx() < 600 || b.Dy() < 600 {
		return false
	}
	transparent, total := 0, 0
	for y := b.Min.Y + b.Dy()/4; y < b.Max.Y-b.Dy()/4; y += 8 {
		for x := b.Min.X + b.Dx()/4; x < b.Max.X-b.Dx()/4; x += 8 {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0x2000 {
				transparent++
			}
			total++
		}
	}
	return total > 0 && transparent*2 > total
}

func (h *MCPHandler) toolUpdateWatermark(c *gin.Context, args map[string]any) (any, bool) {
	id, err := primitive.ObjectIDFromHex(strArg(args, "id"))
	if err != nil {
		return map[string]string{"error": "Invalid watermark ID"}, true
	}
	var name, desc *string
	if v, ok := args["name"].(string); ok {
		name = &v
	}
	if v, ok := args["description"].(string); ok {
		desc = &v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wm, err := updateWatermark(ctx, h.DB, mcpScopeFilter(c), id, name, desc)
	if err != nil {
		return map[string]string{"error": "Watermark not found"}, true
	}
	return wm, false
}

// --- media library ---

type mediaInfo struct {
	models.MediaItem
	AbsoluteURL string `json:"absoluteUrl,omitempty"`
	// TemplateLayers describes a template's layers for compose_image; the
	// design's JSON itself is left out.
	TemplateLayers []templateLayerInfo `json:"templateLayers,omitempty"`
}

func (h *MCPHandler) toolListMedia(c *gin.Context, args map[string]any) (any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	items, err := listMedia(ctx, h.DB, mcpScopeFilter(c), strArg(args, "query"), strSliceArg(args, "tags"), strArg(args, "kind"), intArg(args, "limit", 50))
	if err != nil {
		return map[string]string{"error": "Failed to load media"}, true
	}
	out := make([]mediaInfo, 0, len(items))
	var images []mcpImage
	for _, it := range items {
		info := mediaInfo{MediaItem: it, AbsoluteURL: absoluteURL(c, it.URL)}
		if it.ContentType == models.MediaTemplateType {
			info.TemplateLayers = templateLayerInfos(it.Template)
			info.Template = ""
		}
		out = append(out, info)
		if boolArg(args, "preview") && len(images) < 12 && (strings.HasPrefix(it.ContentType, "image/") || isFontContentType(it.ContentType) || it.ContentType == models.MediaTemplateType) {
			if data, err := readImageBytes(ctx, h.DB, h.UploadDir, it.URL); err == nil {
				if img, err := previewImage(it.ContentType, data); err == nil {
					images = append(images, previewOf(img, 256)...)
				}
			}
		}
	}
	if len(images) > 0 {
		return &mcpRichResult{Result: map[string]any{"media": out, "previews": "attached in list order (images, fonts and template previews only)"}, Images: images}, false
	}
	return out, false
}

func (h *MCPHandler) toolAddMedia(c *gin.Context, args map[string]any) (any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	name, desc, tags := strArg(args, "name"), strArg(args, "description"), strSliceArg(args, "tags")

	if hex := strArg(args, "color"); hex != "" {
		item, err := addMediaColor(ctx, c, h.DB, hex, name, desc, tags)
		if err != nil {
			return map[string]string{"error": err.Error()}, true
		}
		return mediaInfo{MediaItem: item}, false
	}

	if b64 := strArg(args, "data"); b64 != "" {
		filename := strArg(args, "filename")
		if filename == "" {
			return map[string]string{"error": "filename is required with data"}, true
		}
		data, err := base64.StdEncoding.DecodeString(cleanBase64(b64))
		if err != nil {
			return map[string]string{"error": "invalid base64 data"}, true
		}
		if name == "" {
			name = strings.TrimSuffix(filename, filepath.Ext(filename))
		}
		item, err := addMediaItem(ctx, c, h.DB, h.UploadDir, data, filepath.Ext(filename), name, desc, tags)
		if err != nil {
			return map[string]string{"error": err.Error()}, true
		}
		return mediaInfo{MediaItem: item, AbsoluteURL: absoluteURL(c, item.URL)}, false
	}

	raw := strArg(args, "url")
	if raw == "" {
		return map[string]string{"error": "give url, data or color"}, true
	}
	data, src, err := h.loadImageBytes(ctx, c, map[string]any{"url": raw})
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	ct := http.DetectContentType(data)
	if strings.HasPrefix(src, uploadURLPrefix) {
		// Already one of our uploads: reference it instead of copying it.
		if ext := strings.ToLower(filepath.Ext(src)); mediaContentTypes[ext] != "" {
			ct = mediaContentTypes[ext]
		}
		if isFontContentType(ct) {
			if _, ct, err = checkFontUpload(src, data); err != nil {
				return map[string]string{"error": err.Error()}, true
			}
		}
		item, err := recordMediaItem(ctx, c, h.DB, src, data, ct, name, desc, tags)
		if err != nil {
			return map[string]string{"error": "Failed to save media"}, true
		}
		return mediaInfo{MediaItem: item, AbsoluteURL: absoluteURL(c, item.URL)}, false
	}
	ext := ""
	for e, t := range mediaContentTypes {
		if t == ct && e != ".jpeg" {
			ext = e
		}
	}
	if ext == "" {
		return map[string]string{"error": "unsupported file type " + ct}, true
	}
	item, err := addMediaItem(ctx, c, h.DB, h.UploadDir, data, ext, name, desc, tags)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	return mediaInfo{MediaItem: item, AbsoluteURL: absoluteURL(c, item.URL)}, false
}

func (h *MCPHandler) toolUpdateMedia(c *gin.Context, args map[string]any) (any, bool) {
	id, err := primitive.ObjectIDFromHex(strArg(args, "id"))
	if err != nil {
		return map[string]string{"error": "Invalid media ID"}, true
	}
	var name, desc, hex *string
	if v, ok := args["color"].(string); ok {
		hex = &v
	}
	if v, ok := args["name"].(string); ok {
		name = &v
	}
	if v, ok := args["description"].(string); ok {
		desc = &v
	}
	_, setTags := args["tags"]
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	item, err := updateMediaItem(ctx, h.DB, mcpScopeFilter(c), id, name, desc, hex, strSliceArg(args, "tags"), setTags)
	if err == mongo.ErrNoDocuments {
		return map[string]string{"error": "Media not found"}, true
	}
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	return mediaInfo{MediaItem: item, AbsoluteURL: absoluteURL(c, item.URL)}, false
}

func (h *MCPHandler) toolDeleteMedia(c *gin.Context, args map[string]any) (any, bool) {
	id, err := primitive.ObjectIDFromHex(strArg(args, "id"))
	if err != nil {
		return map[string]string{"error": "Invalid media ID"}, true
	}
	filter := mcpScopeFilter(c)
	filter["_id"] = id
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	urls, err := deleteOneAndCollectUploadURLs(ctx, h.DB.Media(), filter)
	if err != nil {
		return map[string]string{"error": "Media not found"}, true
	}
	cleanupUploads(h.DB, h.UploadDir, urls)
	return map[string]string{"message": "Media deleted"}, false
}

// --- get_image ---

func (h *MCPHandler) toolGetImage(c *gin.Context, args map[string]any) (any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	data, src, err := h.loadImageBytes(ctx, c, args)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	img, err := previewImage(http.DetectContentType(data), data)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	maxSide := intArg(args, "maxSide", 768)
	if maxSide > 1536 {
		maxSide = 1536
	}
	b := img.Bounds()
	return &mcpRichResult{
		Result: map[string]any{
			"url":         src,
			"absoluteUrl": absoluteURL(c, src),
			"width":       b.Dx(),
			"height":      b.Dy(),
			"bytes":       len(data),
		},
		Images: previewOf(img, maxSide),
	}, false
}

// --- compose_image ---

func floatArg(args map[string]any, key string) float64 {
	v, _ := args[key].(float64)
	return v
}

// canvasSize resolves format/width/height for a base of size bw x bh (0 when
// there is no base).
func canvasSize(format string, width, height, bw, bh int) (int, int, error) {
	if width > 0 && height > 0 {
		return width, height, nil
	}
	w := 1080
	if width > 0 {
		w = width
	}
	switch format {
	case "", "4:5":
		return w, w * 5 / 4, nil
	case "1:1":
		return w, w, nil
	case "9:16":
		return w, w * 16 / 9, nil
	case "16:9":
		if width == 0 {
			w = 1920
		}
		return w, w * 9 / 16, nil
	case "1.91:1":
		return w, int(float64(w) / 1.91), nil
	case "original":
		if bw == 0 || bh == 0 {
			return w, w, nil
		}
		if width == 0 {
			// Longest side 1080.
			if bw >= bh {
				return 1080, 1080 * bh / bw, nil
			}
			return 1080 * bw / bh, 1080, nil
		}
		return w, w * bh / bw, nil
	}
	return 0, 0, fmt.Errorf("unknown format %q", format)
}

func colorArg(args map[string]any, key string) (color.RGBA, error) {
	s := strArg(args, key)
	if s == "" {
		return color.RGBA{}, nil
	}
	c, ok := services.ParseColor(s)
	if !ok {
		return color.RGBA{}, fmt.Errorf("%s: %q is not a colour (use #rrggbb or #rrggbbaa)", key, s)
	}
	return c, nil
}

// mediaFont loads a font from the media library.
func (h *MCPHandler) mediaFont(ctx context.Context, c *gin.Context, id string) (*opentype.Font, error) {
	data, _, err := h.loadImageBytes(ctx, c, map[string]any{"mediaId": id})
	if err != nil {
		return nil, err
	}
	fonts, err := services.ParseFont(data)
	if err != nil {
		return nil, fmt.Errorf("media %s is not a usable font", id)
	}
	return fonts.Regular, nil
}

// fontSampleText shows the letters, umlauts, digits and the house-style
// quotes a post image is likely to need.
const fontSampleText = "Aa Bb Cc Gg Qq Rr\nÄÖÜ äöü ß 0123456789\n\"Wingspan\" & Co.?!"

// previewImage decodes an image, or for a font renders a text sample in it,
// so agents can look at a library font like at any image.
func previewImage(contentType string, data []byte) (image.Image, error) {
	if !isFontContentType(contentType) {
		return decodeImage(data)
	}
	fonts, err := services.ParseFont(data)
	if err != nil {
		return nil, err
	}
	return services.ComposeImage(services.ComposeSpec{
		Width: 1080, Height: 540,
		Background: color.RGBA{255, 255, 255, 255},
		Layers: []services.ComposeLayer{{
			Kind: "text", Anchor: "center", Align: "center", Width: 0.9,
			Text: fontSampleText, Font: fonts.Regular, FontSize: 0.07,
			Color: color.RGBA{0, 0, 0, 255},
		}},
	})
}

func (h *MCPHandler) buildComposeSpec(ctx context.Context, c *gin.Context, args map[string]any) (services.ComposeSpec, error) {
	var spec services.ComposeSpec
	var bw, bh int
	if base, ok := args["base"].(map[string]any); ok && len(base) > 0 {
		img, err := h.loadImage(ctx, c, base)
		if err != nil {
			return spec, fmt.Errorf("base: %w", err)
		}
		spec.Base = img
		bw, bh = img.Bounds().Dx(), img.Bounds().Dy()
	}
	var w, hgt int
	var err error
	if tpl, ok := args["template"].(map[string]any); ok && len(tpl) > 0 {
		// The template sets the canvas; format, width and height are ignored.
		if spec.Template, err = h.buildTemplate(ctx, c, tpl); err != nil {
			return spec, err
		}
		w, hgt, spec.TemplateScale = templateCanvas(spec.Template)
	} else if w, hgt, err = canvasSize(strArg(args, "format"), intArg(args, "width", 0), intArg(args, "height", 0), bw, bh); err != nil {
		return spec, err
	}
	if w > services.MaxComposeSide || hgt > services.MaxComposeSide {
		return spec, fmt.Errorf("the canvas is limited to %d px per side", services.MaxComposeSide)
	}
	spec.Width, spec.Height = w, hgt
	spec.BaseFit = strArg(args, "fit")
	spec.FocusX, spec.FocusY = floatArg(args, "focusX"), floatArg(args, "focusY")
	if spec.Background, err = colorArg(args, "background"); err != nil {
		return spec, err
	}
	spec.Filter = strArg(args, "filter")
	spec.Brightness = floatArg(args, "brightness")
	spec.Contrast = floatArg(args, "contrast")
	spec.Saturation = floatArg(args, "saturation")

	layers, _ := args["layers"].([]any)
	if len(layers) > 30 {
		return spec, fmt.Errorf("at most 30 layers")
	}
	mediaFonts := map[string]*opentype.Font{}
	for i, raw := range layers {
		la, ok := raw.(map[string]any)
		if !ok {
			return spec, fmt.Errorf("layers[%d] must be an object", i)
		}
		l := services.ComposeLayer{
			Kind:         strArg(la, "type"),
			Anchor:       strArg(la, "anchor"),
			Margin:       floatArg(la, "margin"),
			Width:        floatArg(la, "width"),
			Height:       floatArg(la, "height"),
			Opacity:      floatArg(la, "opacity"),
			Text:         strArg(la, "text"),
			FontSize:     floatArg(la, "fontSize"),
			Align:        strArg(la, "align"),
			MaxLines:     intArg(la, "maxLines", 0),
			OutlineWidth: floatArg(la, "outlineWidth"),
			Shadow:       boolArg(la, "shadow"),
			BoxPadding:   floatArg(la, "boxPadding"),
			Radius:       floatArg(la, "radius"),
		}
		_, hasX := la["x"]
		_, hasY := la["y"]
		if hasX || hasY {
			l.HasXY = true
			l.X, l.Y = floatArg(la, "x"), floatArg(la, "y")
		}
		for key, dst := range map[string]*color.RGBA{"color": &l.Color, "outlineColor": &l.OutlineColor, "boxColor": &l.BoxColor} {
			col, err := colorArg(la, key)
			if err != nil {
				return spec, fmt.Errorf("layers[%d].%w", i, err)
			}
			*dst = col
		}
		switch l.Kind {
		case "image":
			img, err := h.loadImage(ctx, c, la)
			if err != nil {
				return spec, fmt.Errorf("layers[%d]: %w", i, err)
			}
			l.Image = img
		case "text":
			switch strArg(la, "font") {
			case "regular":
				f, err := services.DefaultRegularFont()
				if err != nil {
					return spec, err
				}
				l.Font = f
			}
			if id := strArg(la, "fontMediaId"); id != "" {
				f, ok := mediaFonts[id]
				if !ok {
					var err error
					if f, err = h.mediaFont(ctx, c, id); err != nil {
						return spec, fmt.Errorf("layers[%d]: %w", i, err)
					}
					mediaFonts[id] = f
				}
				l.Font = f
			}
		case "rect", "gradient":
		default:
			return spec, fmt.Errorf("layers[%d]: type must be image, text, rect or gradient", i)
		}
		spec.Layers = append(spec.Layers, l)
	}
	return spec, nil
}

func (h *MCPHandler) toolComposeImage(c *gin.Context, args map[string]any) (any, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	spec, err := h.buildComposeSpec(ctx, c, args)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	img, err := services.ComposeImage(spec)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	data, err := services.EncodeJPEG(img)
	if err != nil {
		return map[string]string{"error": "Failed to encode the image"}, true
	}
	u, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("%d.jpg", time.Now().UnixNano()), "image/jpeg", data)
	if err != nil {
		return map[string]string{"error": "Failed to store the image"}, true
	}
	result := map[string]any{
		"url":         u,
		"absoluteUrl": absoluteURL(c, u),
		"width":       spec.Width,
		"height":      spec.Height,
		"note":        "Not attached to anything yet: pass url in imageUrls of create_post/update_post. Unused images are swept after a day.",
	}
	if boolArg(args, "addToMedia") {
		item, err := recordMediaItem(ctx, c, h.DB, u, data, "image/jpeg", strArg(args, "name"), "Composed image", strSliceArg(args, "tags"))
		if err == nil {
			result["mediaId"] = item.ID.Hex()
			result["note"] = "Added to the media library; pass url in imageUrls of create_post/update_post."
		}
	}
	return &mcpRichResult{Result: result, Images: previewOf(img, 768)}, false
}
