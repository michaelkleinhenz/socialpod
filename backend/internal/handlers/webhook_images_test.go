package handlers

import (
	"context"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"socialmedia/internal/models"
)

func TestMergeImageURLs(t *testing.T) {
	existing := []string{"/api/uploads/a.jpg"}
	saved := []string{"/api/uploads/b.jpg"}

	merged := mergeImageURLs(existing, saved)
	if strings.Join(merged, ",") != "/api/uploads/a.jpg,/api/uploads/b.jpg" {
		t.Fatalf("unexpected merge result: %v", merged)
	}
	if len(existing) != 1 || existing[0] != "/api/uploads/a.jpg" {
		t.Fatalf("existing slice was mutated: %v", existing)
	}

	if got := mergeImageURLs(nil, nil); len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
}

// webhookImageNames collects the filenames of the "image" parts a webhook
// receives, so tests can assert which images were forwarded.
func webhookImageNames(t *testing.T, r *http.Request) []string {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse content type: %v", err)
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	var names []string
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		if part.FormName() == "image" {
			names = append(names, part.FileName())
		}
	}
	return names
}

func writeUpload(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("image-bytes"), 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	return "/api/uploads/" + name
}

// A news draft loaded back into the form supplies its image as a stored URL
// rather than a fresh upload; the n8n webhook must still receive it.
func TestSendToN8NForwardsStoredImageURLs(t *testing.T) {
	dir := t.TempDir()
	storedURL := writeUpload(t, dir, "stored.jpg")

	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = webhookImageNames(t, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := &NewsHandler{UploadDir: dir}
	team := &models.Team{NewsCreatorURL: srv.URL, NewsCreatorBearerToken: "token"}
	input := &NewsSubmitInput{EpisodeNumber: "42", NewsTagline: "Tagline", ArticleURL: "https://example.com"}

	if err := h.sendToN8N(context.Background(), team, input, []string{storedURL}); err != nil {
		t.Fatalf("sendToN8N: %v", err)
	}
	if len(received) != 1 || received[0] != "stored.jpg" {
		t.Fatalf("expected stored.jpg to be forwarded, got %v", received)
	}
}

func TestSendToWebhookForwardsStoredImageURLs(t *testing.T) {
	dir := t.TempDir()
	storedURL := writeUpload(t, dir, "cover.png")

	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = webhookImageNames(t, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := &EpisodeHandler{UploadDir: dir}
	team := &models.Team{EpisodeCreatorURL: srv.URL, EpisodeCreatorBearerToken: "token"}
	input := &EpisodeSubmitInput{EpisodeNumber: "7", EpisodeTitle: "Title", EpisodeType: "news", EpisodeDate: "2026-01-01"}

	if err := h.sendToWebhook(context.Background(), team, input, []string{storedURL}); err != nil {
		t.Fatalf("sendToWebhook: %v", err)
	}
	if len(received) != 1 || received[0] != "cover.png" {
		t.Fatalf("expected cover.png to be forwarded, got %v", received)
	}
}
