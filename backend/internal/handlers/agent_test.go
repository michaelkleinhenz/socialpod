package handlers

import "testing"

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
