package handlers

import (
	"strings"
	"testing"
)

func TestExtractMetaContent_PropertyBeforeContent(t *testing.T) {
	html := `<html><head><meta property="og:image" content="https://example.com/img.jpg" /></head></html>`
	got := extractMetaContent(html, "og:image")
	if got != "https://example.com/img.jpg" {
		t.Fatalf("expected og:image URL, got %q", got)
	}
}

func TestExtractMetaContent_ContentBeforeProperty(t *testing.T) {
	html := `<html><head><meta content="https://example.com/img.jpg" property="og:image" /></head></html>`
	got := extractMetaContent(html, "og:image")
	if got != "https://example.com/img.jpg" {
		t.Fatalf("expected og:image URL, got %q", got)
	}
}

func TestExtractMetaContent_NameAttribute(t *testing.T) {
	html := `<html><head><meta name="description" content="Hello world" /></head></html>`
	got := extractMetaContent(html, "description")
	if got != "Hello world" {
		t.Fatalf("expected description, got %q", got)
	}
}

func TestExtractMetaContent_MultipleNames(t *testing.T) {
	html := `<html><head><meta property="og:description" content="OG desc" /></head></html>`
	got := extractMetaContent(html, "description", "og:description")
	if got != "OG desc" {
		t.Fatalf("expected OG desc, got %q", got)
	}
}

func TestExtractMetaContent_NotFound(t *testing.T) {
	html := `<html><head><meta property="og:title" content="Title" /></head></html>`
	got := extractMetaContent(html, "og:image")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractMetaContent_DoubleQuotesAndSingle(t *testing.T) {
	html := `<html><head><meta property='og:image' content='https://example.com/img.png' /></head></html>`
	got := extractMetaContent(html, "og:image")
	if got != "https://example.com/img.png" {
		t.Fatalf("expected og:image URL, got %q", got)
	}
}

func TestExtractMetaContent_MixedQuotes(t *testing.T) {
	html := `<html><head><meta content="https://example.com/img.jpg" property='og:image' /></head></html>`
	got := extractMetaContent(html, "og:image")
	if got != "https://example.com/img.jpg" {
		t.Fatalf("expected og:image URL, got %q", got)
	}
}

func TestExtractMetaContent_CaseInsensitive(t *testing.T) {
	html := `<html><head><META PROPERTY="OG:IMAGE" CONTENT="https://example.com/img.jpg" /></head></html>`
	got := extractMetaContent(html, "og:image")
	if got != "https://example.com/img.jpg" {
		t.Fatalf("expected og:image URL, got %q", got)
	}
}

func TestResolveURL_Absolute(t *testing.T) {
	got := resolveURL("https://example.com/img.jpg", "https://other.com/page")
	if got != "https://example.com/img.jpg" {
		t.Fatalf("expected absolute URL unchanged, got %q", got)
	}
}

func TestResolveURL_Relative(t *testing.T) {
	got := resolveURL("/images/photo.jpg", "https://example.com/news/article")
	if got != "https://example.com/images/photo.jpg" {
		t.Fatalf("expected resolved URL, got %q", got)
	}
}

func TestResolveURL_ProtocolRelative(t *testing.T) {
	got := resolveURL("//cdn.example.com/img.jpg", "https://example.com/page")
	if got != "https://cdn.example.com/img.jpg" {
		t.Fatalf("expected https protocol, got %q", got)
	}
}

func TestResolveURL_Empty(t *testing.T) {
	got := resolveURL("", "https://example.com/page")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractContentImageCandidates_OGImage(t *testing.T) {
	html := `<html><head><meta property="og:image" content="https://example.com/og.jpg" /></head><body></body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/page")
	if len(candidates) == 0 || candidates[0] != "https://example.com/og.jpg" {
		t.Fatalf("expected og:image as first candidate, got %v", candidates)
	}
}

func TestExtractContentImageCandidates_TwitterFallback(t *testing.T) {
	html := `<html><head><meta name="twitter:image" content="https://example.com/tw.jpg" /></head><body></body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/page")
	if len(candidates) == 0 || candidates[0] != "https://example.com/tw.jpg" {
		t.Fatalf("expected twitter:image as first candidate, got %v", candidates)
	}
}

func TestExtractContentImageCandidates_JSONLD(t *testing.T) {
	html := `<html><head><script type="application/ld+json">{"@type":"Article","image":"https://example.com/ld.jpg"}</script></head><body></body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/page")
	if len(candidates) == 0 || candidates[0] != "https://example.com/ld.jpg" {
		t.Fatalf("expected JSON-LD image as candidate, got %v", candidates)
	}
}

func TestExtractContentImageCandidates_JSONLDGraph(t *testing.T) {
	html := `<html><head><script type="application/ld+json">{"@graph":[{"@type":"WebPage"},{"@type":"Article","image":{"url":"https://example.com/graph.jpg"}}]}</script></head><body></body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/page")
	if len(candidates) == 0 || candidates[0] != "https://example.com/graph.jpg" {
		t.Fatalf("expected JSON-LD @graph image, got %v", candidates)
	}
}

func TestExtractContentImageCandidates_ImgTagFallback(t *testing.T) {
	html := `<html><body>
		<img src="https://example.com/small-icon.png" width="16" height="16" alt="">
		<img src="https://example.com/article-hero.jpg" width="800" height="600" alt="Product announcement banner">
		<img src="data:image/gif;base64,R0lGODlhAQABAIAAAA==" alt="">
	</body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/page")
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate from img tags")
	}
	if candidates[0] != "https://example.com/article-hero.jpg" {
		t.Fatalf("expected hero image as top candidate, got %v", candidates)
	}
}

func TestExtractContentImageCandidates_SkipsDataURIs(t *testing.T) {
	html := `<html><body>
		<img src="data:image/svg+xml,%3csvg%20xmlns=%27http://www.w3.org/2000/svg%27%20width=%2740%27%20height=%2740%27/%3e">
		<img src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7">
	</body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/page")
	for _, c := range candidates {
		if strings.HasPrefix(c, "data:") {
			t.Fatalf("data: URI should be excluded, got %q", c)
		}
	}
}

func TestExtractContentImageCandidates_RelativeURLs(t *testing.T) {
	html := `<html><body><img src="/uploads/photo.jpg" width="600" height="400" alt="Photo"></body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/news/article")
	if len(candidates) == 0 {
		t.Fatal("expected candidate from relative URL")
	}
	if candidates[0] != "https://example.com/uploads/photo.jpg" {
		t.Fatalf("expected resolved URL, got %q", candidates[0])
	}
}

func TestExtractContentImageCandidates_NegativePatterns(t *testing.T) {
	html := `<html><body>
		<img src="https://example.com/logo.png" width="200" height="100" alt="Company Logo">
		<img src="https://example.com/article-content.jpg" width="800" height="600" alt="Article content">
	</body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/page")
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	if candidates[0] != "https://example.com/article-content.jpg" {
		t.Fatalf("expected content image ranked above logo, got %v", candidates)
	}
}

func TestExtractContentImageCandidates_AsmodeeStylePage(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="https://assets.svc.asmodee.net/production/undefined/produktbanner_web.png" data-next-head="">
	</head><body>
		<img src="https://assets.svc.asmodee.net/production/undefined/produktbanner_web.png" class="css-1tehys1 e2atz391">
		<img src="https://assets.svc.asmodee.net/production/box-3d.png" class="css-k19fg7" srcset="https://cdn.example.com/box-3d.png?w=350 640w">
		<img alt="" aria-hidden="true" src="data:image/svg+xml,%3csvg%20xmlns=%27http://www.w3.org/2000/svg%27%20width=%27120%27%20height=%2736%27/%3e">
		<img alt="Partner logo" src="https://assets.svc.asmodee.net/production/Logo_Partner.png" class="css-3abrc0">
		<img src="https://cdn.example.com/social_media_image.jpg">
	</body></html>`
	candidates := extractContentImageCandidates(html, "https://www.asmodee.de/news/test")
	if len(candidates) == 0 {
		t.Fatal("expected candidates from Asmodee-style page")
	}
	if candidates[0] != "https://assets.svc.asmodee.net/production/undefined/produktbanner_web.png" {
		t.Fatalf("expected og:image as first candidate, got %v", candidates)
	}
	for _, c := range candidates {
		if strings.HasPrefix(c, "data:") {
			t.Fatalf("data: URI should be excluded, got %q", c)
		}
	}
}

func TestScoreImgTags_Deduplication(t *testing.T) {
	html := `<html><body>
		<img src="https://example.com/same.jpg">
		<img src="https://example.com/same.jpg">
	</body></html>`
	scored := scoreImgTags(html, "https://example.com/page")
	if len(scored) != 1 {
		t.Fatalf("expected 1 deduplicated candidate, got %d", len(scored))
	}
}

func TestExtractJSONLDImage_ImageArray(t *testing.T) {
	html := `<script type="application/ld+json">{"image":["https://example.com/first.jpg","https://example.com/second.jpg"]}</script>`
	got := extractJSONLDImage(html, "https://example.com")
	if got != "https://example.com/first.jpg" {
		t.Fatalf("expected first image from array, got %q", got)
	}
}

func TestExtractJSONLDImage_ImageObject(t *testing.T) {
	html := `<script type="application/ld+json">{"image":{"@type":"ImageObject","url":"https://example.com/obj.jpg"}}</script>`
	got := extractJSONLDImage(html, "https://example.com")
	if got != "https://example.com/obj.jpg" {
		t.Fatalf("expected image from object URL, got %q", got)
	}
}
