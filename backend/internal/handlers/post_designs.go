package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// A post's designs keep the images and videos made in the image editor
// editable (models.PostDesign). The post editor sends one with every image it
// exported from the editor; an MCP client sends them to leave a draft that
// is finished in the editor, which also lets it hand over a video it cannot
// render (or an image too large to upload) as a design only. A post with a
// design that has not been exported yet cannot be scheduled.

const (
	maxPostDesigns = 10
	// All designs of a post together, well inside MongoDB's document limit.
	maxPostDesignBytes = 8 * 1024 * 1024
)

// postDesignInput is one design as a client sends it.
type postDesignInput struct {
	// ImageURL is the export among the post's images; empty or absent for a
	// design not exported yet.
	ImageURL string `json:"imageUrl,omitempty"`
	// File points at the n-th file sent with this request instead (the
	// multipart images of the REST API, the inline images of MCP), whose URL
	// is only known once it is stored.
	File *int `json:"file,omitempty"`
	// PreviewURL is a still shown for an unexported design.
	PreviewURL string `json:"previewUrl,omitempty"`
	// Design is the edit: an image draft's design ({"template", "photo"}),
	// or a bare template ({"width", "height", "layers", …}), as an object or
	// as a JSON string.
	Design json.RawMessage `json:"design"`
}

// errUnexportedDesign is the answer to scheduling a post whose design has
// not been exported yet.
var errUnexportedDesign = fmt.Errorf("this post has a design that has not been exported yet: open it in the post editor, export it, then schedule the post (until then it can only be saved as a draft)")

// normalizeDesign returns a design as the editor stores it: an image draft's
// JSON. A string is taken as the JSON itself; a bare template is wrapped.
func normalizeDesign(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("design is required")
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("design is not valid JSON: %v", err)
		}
		raw = json.RawMessage(strings.TrimSpace(s))
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", fmt.Errorf("design is not a JSON object: %v", err)
	}
	if _, ok := probe["template"]; ok {
		return string(raw), nil
	}
	if _, ok := probe["layers"]; !ok {
		return "", fmt.Errorf("design must be an image editor design ({template, photo}) or a template ({width, height, background, layers})")
	}
	wrapped, err := json.Marshal(map[string]any{"version": 1, "template": raw})
	if err != nil {
		return "", err
	}
	return string(wrapped), nil
}

// designIsVideo reports whether a design (an image draft's JSON) is a video:
// its template has a length.
func designIsVideo(design string) bool {
	var d struct {
		Template struct {
			Duration float64 `json:"duration"`
		} `json:"template"`
	}
	return json.Unmarshal([]byte(design), &d) == nil && d.Template.Duration > 0
}

// buildPostDesigns checks the designs sent with a post against its images
// (imageURLs, after this request's files are stored as uploaded) and returns
// them as stored, with the uploads they use.
func buildPostDesigns(in []postDesignInput, uploaded, imageURLs []string) ([]models.PostDesign, []string, error) {
	if len(in) > maxPostDesigns {
		return nil, nil, fmt.Errorf("a post holds at most %d designs", maxPostDesigns)
	}
	images := make(map[string]bool, len(imageURLs))
	for _, u := range imageURLs {
		images[u] = true
	}
	designs := make([]models.PostDesign, 0, len(in))
	var urls []string
	seen := map[string]bool{}
	total := 0
	for i, d := range in {
		imageURL := strings.TrimSpace(d.ImageURL)
		if d.File != nil {
			if *d.File < 0 || *d.File >= len(uploaded) {
				return nil, nil, fmt.Errorf("designs[%d].file: there is no file %d in this request", i, *d.File)
			}
			imageURL = uploaded[*d.File]
		}
		if imageURL != "" {
			if !images[imageURL] {
				return nil, nil, fmt.Errorf("designs[%d].imageUrl %q is not one of the post's images", i, imageURL)
			}
			if seen[imageURL] {
				return nil, nil, fmt.Errorf("designs[%d]: the image %q has a design already", i, imageURL)
			}
			seen[imageURL] = true
		}
		design, err := normalizeDesign(d.Design)
		if err != nil {
			return nil, nil, fmt.Errorf("designs[%d]: %v", i, err)
		}
		_, _, used, err := checkImageDraft(design)
		if err != nil {
			return nil, nil, fmt.Errorf("designs[%d]: %v", i, err)
		}
		total += len(design)
		if total > maxPostDesignBytes {
			return nil, nil, fmt.Errorf("the post's designs are larger than %d MB together", maxPostDesignBytes/(1024*1024))
		}
		preview := strings.TrimSpace(d.PreviewURL)
		if imageURL != "" {
			// The export shows the design.
			preview = ""
		}
		if preview != "" && uploadFilename(preview) != "" {
			used = append(used, preview)
		}
		urls = append(urls, used...)
		designs = append(designs, models.PostDesign{
			ImageURL:   imageURL,
			PreviewURL: preview,
			Video:      designIsVideo(design),
			Design:     design,
		})
	}
	return designs, uniqueStrings(urls), nil
}

// keptDesigns are the stored designs that still belong to a post whose
// images became imageURLs: those of an image it still has, and the
// unexported ones.
func keptDesigns(designs []models.PostDesign, imageURLs []string) []models.PostDesign {
	images := make(map[string]bool, len(imageURLs))
	for _, u := range imageURLs {
		images[u] = true
	}
	kept := make([]models.PostDesign, 0, len(designs))
	for _, d := range designs {
		if d.ImageURL == "" || images[d.ImageURL] {
			kept = append(kept, d)
		}
	}
	return kept
}

// designUploadURLs are the uploads designs use, read again from the designs
// themselves (for designs kept from the stored post).
func designUploadURLs(designs []models.PostDesign) []string {
	var urls []string
	for _, d := range designs {
		if _, _, used, err := checkImageDraft(d.Design); err == nil {
			urls = append(urls, used...)
		}
		if d.PreviewURL != "" && uploadFilename(d.PreviewURL) != "" {
			urls = append(urls, d.PreviewURL)
		}
	}
	return uniqueStrings(urls)
}

// hasUnexportedDesign reports whether a design still waits for its export.
func hasUnexportedDesign(designs []models.PostDesign) bool {
	for _, d := range designs {
		if d.ImageURL == "" {
			return true
		}
	}
	return false
}

// setPostDesigns puts designs into an update: $set, or $unset when none
// are left.
func setPostDesigns(set, unset bson.M, designs []models.PostDesign, urls []string) {
	if len(designs) == 0 {
		unset["designs"] = ""
		unset["designUrls"] = ""
		return
	}
	set["designs"] = designs
	if len(urls) == 0 {
		unset["designUrls"] = ""
	} else {
		set["designUrls"] = urls
	}
}

// updatePostDesigns puts into an update of the post filter matches the
// designs it leaves the post with: the ones sent (in), or else the stored
// ones less those of images the update drops (imageURLs, nil when it leaves
// the images alone). It refuses an update that would leave a post that is
// not a draft with an unexported design. A post that is not found is left
// to the update to report.
func updatePostDesigns(ctx context.Context, coll *mongo.Collection, filter bson.M, in *[]postDesignInput, uploaded, imageURLs []string, status *models.PostStatus, set, unset bson.M) error {
	if in == nil && imageURLs == nil && status == nil {
		return nil
	}
	var existing models.Post
	opts := options.FindOne().SetProjection(bson.M{"imageUrls": 1, "status": 1, "designs": 1})
	if err := coll.FindOne(ctx, filter, opts).Decode(&existing); err != nil {
		return nil
	}
	images := existing.ImageURLs
	if imageURLs != nil {
		images = imageURLs
	}
	finalStatus := existing.Status
	if status != nil {
		finalStatus = *status
	}
	designs := existing.Designs
	changed := false
	var urls []string
	if in != nil {
		var err error
		if designs, urls, err = buildPostDesigns(*in, uploaded, images); err != nil {
			return err
		}
		changed = true
	} else if imageURLs != nil {
		if kept := keptDesigns(designs, images); len(kept) != len(designs) {
			designs, urls, changed = kept, designUploadURLs(kept), true
		}
	}
	if finalStatus != models.PostStatusDraft && hasUnexportedDesign(designs) {
		return errUnexportedDesign
	}
	if changed {
		setPostDesigns(set, unset, designs, urls)
	}
	return nil
}
