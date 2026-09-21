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

func TestExtractNextDataImages_Basic(t *testing.T) {
	html := `<html><head><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"product":{"image":{"url":"https://cdn.example.com/product.jpg"}}}}}</script></head></html>`
	got := extractNextDataImages(html, "https://example.com")
	if len(got) == 0 {
		t.Fatal("expected at least one image from __NEXT_DATA__")
	}
	found := false
	for _, u := range got {
		if u == "https://cdn.example.com/product.jpg" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected product.jpg in candidates, got %v", got)
	}
}

func TestExtractNextDataImages_MultipleImages(t *testing.T) {
	html := `<html><head><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"items":[{"image":"https://cdn.example.com/a.jpg"},{"image":"https://cdn.example.com/b.png"}]}}}</script></head></html>`
	got := extractNextDataImages(html, "https://example.com")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 images, got %v", got)
	}
}

func TestExtractNextDataImages_NoScript(t *testing.T) {
	html := `<html><body>no next data here</body></html>`
	got := extractNextDataImages(html, "https://example.com")
	if len(got) != 0 {
		t.Fatalf("expected no results, got %v", got)
	}
}

func TestExtractNextDataImages_AsmodeeStyle(t *testing.T) {
	html := `<html><head><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"news":{"images":[{"url":"https://xr.orbitlabs.de/i/default/SCOD5008/azul-kids.png/resize/3840x/format/webp"}],"external_media_data":[{"file":"https://retail.asmodee.de/media/catalog/product/a/z/azul-kids.jpg"}]}}}}</script></head></html>`
	got := extractNextDataImages(html, "https://www.asmodee.de/news/test")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 images from Asmodee-style __NEXT_DATA__, got %v", got)
	}
}

func TestExtractSrcsetURLs_Basic(t *testing.T) {
	html := `<html><body><img src="small.jpg" srcset="https://cdn.example.com/img-350w.jpg 350w, https://cdn.example.com/img-800w.jpg 800w, https://cdn.example.com/img-1200w.jpg 1200w"></body></html>`
	got := extractSrcsetURLs(html, "https://example.com")
	if len(got) == 0 {
		t.Fatal("expected at least one srcset URL")
	}
	if got[0] != "https://cdn.example.com/img-1200w.jpg" {
		t.Fatalf("expected largest srcset image, got %q", got[0])
	}
}

func TestExtractSrcsetURLs_NoSrcset(t *testing.T) {
	html := `<html><body><img src="https://example.com/img.jpg"></body></html>`
	got := extractSrcsetURLs(html, "https://example.com")
	if len(got) != 0 {
		t.Fatalf("expected no results, got %v", got)
	}
}

func TestUnwrapNextImageURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain URL", "https://example.com/img.jpg", "https://example.com/img.jpg"},
		{"next/image URL", "https://example.com/_next/image?url=https%3A%2F%2Fcdn.example.com%2Fphoto.jpg&w=1200&q=75", "https://cdn.example.com/photo.jpg"},
		{"next/image relative", "/_next/image?url=%2Fuploads%2Fphoto.jpg&w=640&q=75", "/uploads/photo.jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unwrapNextImageURL(tt.input)
			if got != tt.expected {
				t.Fatalf("unwrapNextImageURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractContentImageCandidates_NextDataFallback(t *testing.T) {
	html := `<html><head>
		<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"article":{"image":"https://cdn.example.com/article-hero.jpg"}}}}</script>
	</head><body>
		<img src="data:image/svg+xml,%3csvg%20xmlns=%27http://www.w3.org/2000/svg%27%20width=%2740%27%20height=%2740%27/%3e">
	</body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/page")
	if len(candidates) == 0 {
		t.Fatal("expected __NEXT_DATA__ image as candidate when no og:image")
	}
	if candidates[0] != "https://cdn.example.com/article-hero.jpg" {
		t.Fatalf("expected __NEXT_DATA__ image, got %v", candidates)
	}
}

func TestExtractContentImageCandidates_SrcsetFallback(t *testing.T) {
	html := `<html><body>
		<img src="data:image/gif;base64,R0lGOD" srcset="https://cdn.example.com/hero-640w.jpg 640w, https://cdn.example.com/hero-1920w.jpg 1920w">
	</body></html>`
	candidates := extractContentImageCandidates(html, "https://example.com/page")
	found := false
	for _, c := range candidates {
		if c == "https://cdn.example.com/hero-1920w.jpg" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected largest srcset image in candidates, got %v", candidates)
	}
}

func TestBGGGameIDFromURL_GamePage(t *testing.T) {
	if got := bggGameIDFromURL("https://boardgamegeek.com/boardgame/224517/brass-birmingham"); got != "224517" {
		t.Errorf("expected 224517, got %q", got)
	}
}

func TestBGGGameIDFromURL_WWWAndExpansion(t *testing.T) {
	if got := bggGameIDFromURL("https://www.boardgamegeek.com/boardgameexpansion/161936/pandemic-legacy-season-1"); got != "161936" {
		t.Errorf("expected 161936, got %q", got)
	}
}

func TestBGGGameIDFromURL_ExtraPathSegments(t *testing.T) {
	if got := bggGameIDFromURL("https://boardgamegeek.com/de/boardgame/13/catan?foo=bar#comments"); got != "13" {
		t.Errorf("expected 13, got %q", got)
	}
}

func TestBGGGameIDFromURL_SchemeLess(t *testing.T) {
	if got := bggGameIDFromURL("boardgamegeek.com/boardgame/13/catan"); got != "13" {
		t.Errorf("expected 13, got %q", got)
	}
}

func TestBGGGameIDFromURL_OtherHostWithBGGLikePath(t *testing.T) {
	if got := bggGameIDFromURL("https://example.com/boardgame/224517/brass-birmingham"); got != "" {
		t.Errorf("expected no match for non-BGG host, got %q", got)
	}
}

func TestBGGGameIDFromURL_LookalikeHost(t *testing.T) {
	if got := bggGameIDFromURL("https://notboardgamegeek.com/boardgame/224517/x"); got != "" {
		t.Errorf("expected no match for look-alike host, got %q", got)
	}
}

func TestBGGGameIDFromURL_NonGamePage(t *testing.T) {
	if got := bggGameIDFromURL("https://boardgamegeek.com/thread/123456/some-thread"); got != "" {
		t.Errorf("expected no match for non-game page, got %q", got)
	}
}

func TestBGGGameIDFromURL_Empty(t *testing.T) {
	if got := bggGameIDFromURL(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestBGGOverlayType_MatchesUIForms(t *testing.T) {
	// News page and post editor call /bgg/fetch without an episodeType, which
	// selects the team's BGG watermark; the Episode page sends its type
	// selector, which defaults to "news".
	if got := bggOverlayType("news"); got != "" {
		t.Errorf("news: expected empty episodeType, got %q", got)
	}
	if got := bggOverlayType("post"); got != "" {
		t.Errorf("post: expected empty episodeType, got %q", got)
	}
	if got := bggOverlayType("episode"); got != "news" {
		t.Errorf("episode: expected \"news\", got %q", got)
	}
}
