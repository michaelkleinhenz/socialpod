package handlers

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscriptWordsUsesWordTimestamps(t *testing.T) {
	got := transcriptWords(transcriptionResponse{
		Words: []TranscriptWord{{Word: " Hallo", Start: 0.2, End: 0.5}, {Word: " ", Start: 0.5, End: 0.6}, {Word: "Welt", Start: 0.7, End: 0.6}},
	})
	if len(got) != 2 || got[0].Word != "Hallo" || got[1].Word != "Welt" {
		t.Fatalf("words = %+v", got)
	}
	if got[1].End != 0.7 {
		t.Errorf("an end before the start is clamped to it, got %v", got[1].End)
	}
}

func TestTranscriptWordsSpreadsSegments(t *testing.T) {
	var r transcriptionResponse
	json.Unmarshal([]byte(`{"segments":[{"start":1,"end":3,"text":" ab cd "},{"start":4,"end":4,"text":"skipped"}]}`), &r)
	got := transcriptWords(r)
	if len(got) != 2 {
		t.Fatalf("words = %+v", got)
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if !near(got[0].Start, 1) || !near(got[0].End, 2) || !near(got[1].Start, 2) || !near(got[1].End, 3) {
		t.Errorf("words = %+v, want ab 1–2 and cd 2–3", got)
	}
}

func TestTranscribeFallsBackToTheNextModel(t *testing.T) {
	var models []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(1 << 20)
		models = append(models, r.FormValue("model"))
		if got := r.MultipartForm.Value["timestamp_granularities[]"]; len(got) != 2 || got[0] != "word" {
			t.Errorf("granularities = %v", got)
		}
		if r.FormValue("language") != "de" {
			t.Errorf("language = %q", r.FormValue("language"))
		}
		if len(models) == 1 {
			http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"text":"Hallo","language":"german","words":[{"word":"Hallo","start":0,"end":0.4}]}`))
	}))
	defer srv.Close()
	old := openRouterTranscribeURL
	openRouterTranscribeURL = srv.URL
	defer func() { openRouterTranscribeURL = old }()

	resp, err := transcribe(context.Background(), "key", []byte("RIFF"), "a.wav", "de")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != transcribeModels[0] || models[1] != transcribeModels[1] {
		t.Errorf("models asked = %v", models)
	}
	if resp.Text != "Hallo" || len(resp.Words) != 1 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestTranscribeStopsOnAuthErrors(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer srv.Close()
	old := openRouterTranscribeURL
	openRouterTranscribeURL = srv.URL
	defer func() { openRouterTranscribeURL = old }()

	if _, err := transcribe(context.Background(), "key", []byte("RIFF"), "a.wav", ""); err == nil {
		t.Fatal("want an error")
	}
	if calls != 1 {
		t.Errorf("a bad key is not retried with another model, got %d calls", calls)
	}
}
