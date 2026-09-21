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

	"go.mongodb.org/mongo-driver/bson"
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

func TestApplyPostedFilter(t *testing.T) {
	open := bson.M{"teamId": "t"}
	applyPostedFilter(open, false)
	cond, ok := open["posted"].(bson.M)
	if !ok || cond["$ne"] != true {
		// Drafts written before posting was tracked carry no "posted" field,
		// so they must still show up as open drafts.
		t.Fatalf("expected open drafts to match a missing posted field, got %v", open["posted"])
	}

	posted := bson.M{"teamId": "t"}
	applyPostedFilter(posted, true)
	if posted["posted"] != true {
		t.Fatalf("expected posted filter, got %v", posted["posted"])
	}

	if key := postedSort(true)[0].Key; key != "postedAt" {
		t.Fatalf("expected posted list sorted by postedAt, got %q", key)
	}
	if key := postedSort(false)[0].Key; key != "updatedAt" {
		t.Fatalf("expected draft list sorted by updatedAt, got %q", key)
	}
}

// Submitting a draft stores the content it was submitted with, so the Posted
// tab shows what actually went out rather than the last saved draft.
func TestDraftFieldsCoverSubmittedContent(t *testing.T) {
	news := newsDraftFields(&NewsSubmitInput{
		EpisodeNumber: "42",
		NewsTagline:   "Edited tagline",
		ArticleURL:    "https://example.com",
		Content:       "Edited content",
	}, []string{"/api/uploads/new.jpg"})

	if news["newsTagline"] != "Edited tagline" || news["content"] != "Edited content" {
		t.Fatalf("submitted news content not recorded: %v", news)
	}
	if urls, _ := news["imageUrls"].([]string); len(urls) != 1 || urls[0] != "/api/uploads/new.jpg" {
		t.Fatalf("submitted news image not recorded: %v", news["imageUrls"])
	}

	episode := episodeDraftFields(&EpisodeSubmitInput{
		EpisodeNumber: "7",
		EpisodeTitle:  "Edited title",
		EpisodeType:   "review",
		EpisodeDate:   "2026-01-01",
	}, []string{"/api/uploads/cover.png"})

	if episode["episodeTitle"] != "Edited title" || episode["episodeType"] != "review" {
		t.Fatalf("submitted episode content not recorded: %v", episode)
	}
	if urls, _ := episode["imageUrls"].([]string); len(urls) != 1 || urls[0] != "/api/uploads/cover.png" {
		t.Fatalf("submitted episode image not recorded: %v", episode["imageUrls"])
	}
}
