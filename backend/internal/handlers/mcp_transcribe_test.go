package handlers

import "testing"

func TestTranscriptPhrases(t *testing.T) {
	got := transcriptPhrases([]TranscriptWord{
		{Word: "Hallo", Start: 0, End: 0.3},
		{Word: "zusammen.", Start: 0.35, End: 0.8},
		{Word: "Heute", Start: 0.9, End: 1.1},
		{Word: "spielen", Start: 1.15, End: 1.4},
		{Word: "wir", Start: 2.5, End: 2.7},
	})
	want := []TranscriptPhrase{
		{Text: "Hallo zusammen.", Start: 0, End: 0.8},
		{Text: "Heute spielen", Start: 0.9, End: 1.4},
		{Text: "wir", Start: 2.5, End: 2.7},
	}
	if len(got) != len(want) {
		t.Fatalf("phrases = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("phrase %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestTranscribeFilename(t *testing.T) {
	cases := map[string]string{
		"/api/uploads/abc.mov":             "audio.mp4",
		"https://example.com/a/clip.MP4?x": "audio.mp4",
		"/api/uploads/sound.aac":           "audio.m4a",
		"/api/uploads/voice.wav":           "audio.wav",
	}
	for src, want := range cases {
		if got, err := transcribeFilename(src, nil); err != nil || got != want {
			t.Errorf("transcribeFilename(%q) = %q, %v; want %q", src, got, err, want)
		}
	}
	if got, err := transcribeFilename("https://example.com/x", []byte("ID3\x04\x00")); err != nil || got != "audio.mp3" {
		t.Errorf("sniffed mp3 = %q, %v", got, err)
	}
	if _, err := transcribeFilename("/api/uploads/a.jpg", []byte("\xff\xd8\xff\xe0")); err == nil {
		t.Error("an image is refused")
	}
}
