package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// TranscribeHandler turns speech into timed words, for the image editor's
// auto-captions on a video.
type TranscribeHandler struct {
	DB *database.MongoDB
}

// openRouterTranscribeURL is OpenRouter's speech-to-text endpoint, the
// OpenAI-compatible one; a variable so tests can point it elsewhere.
var openRouterTranscribeURL = "https://openrouter.ai/api/v1/audio/transcriptions"

// transcribeModels are tried in order: a model OpenRouter does not offer (or
// that refuses word timestamps) answers 4xx, and the next one is asked.
var transcribeModels = []string{"openai/whisper-1", "openai/whisper-large-v3"}

// maxTranscribeBytes is OpenRouter's limit for an upload; the editor sends
// 16 kHz mono WAV, about 2.9 MB for the longest (90 s) video.
const maxTranscribeBytes = 25 << 20

var languageCodeRe = regexp.MustCompile(`^[a-z]{2}$`)

// TranscriptWord is one spoken word and when it is said, in seconds of the
// audio sent.
type TranscriptWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// transcriptionResponse is the verbose_json answer of an OpenAI-compatible
// transcription: words when the provider gives word timestamps, segments in
// any case.
type transcriptionResponse struct {
	Text     string           `json:"text"`
	Language string           `json:"language"`
	Words    []TranscriptWord `json:"words"`
	Segments []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
	} `json:"segments"`
}

// transcriptWords returns the response's words, trimmed, the empty ones left
// out. A provider that gives segments only has their words spread over each
// segment by length, which is close enough for captions of a few words.
func transcriptWords(r transcriptionResponse) []TranscriptWord {
	out := []TranscriptWord{}
	if len(r.Words) > 0 {
		for _, w := range r.Words {
			if t := strings.TrimSpace(w.Word); t != "" {
				out = append(out, TranscriptWord{Word: t, Start: w.Start, End: max(w.End, w.Start)})
			}
		}
		return out
	}
	for _, s := range r.Segments {
		words := strings.Fields(s.Text)
		total := 0
		for _, w := range words {
			total += utf8.RuneCountInString(w) + 1
		}
		if total == 0 || s.End <= s.Start {
			continue
		}
		per := (s.End - s.Start) / float64(total)
		at := s.Start
		for _, w := range words {
			length := float64(utf8.RuneCountInString(w)+1) * per
			out = append(out, TranscriptWord{Word: w, Start: at, End: at + length})
			at += length
		}
	}
	return out
}

// Transcribe sends an audio file to OpenRouter's Whisper and returns the
// words with their times.
// POST /api/transcribe (multipart: audio, optional language as ISO 639-1)
func (h *TranscribeHandler) Transcribe(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxTranscribeBytes+1<<20)
	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No audio sent"})
		return
	}
	defer file.Close()
	if header.Size > maxTranscribeBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "The audio is too long to transcribe"})
		return
	}
	audio, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read the audio"})
		return
	}
	language := strings.ToLower(strings.TrimSpace(c.PostForm("language")))
	if language != "" && !languageCodeRe.MatchString(language) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown language"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil || settings.OpenRouterAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OpenRouter is not configured"})
		return
	}

	name := header.Filename
	if name == "" {
		name = "audio.wav"
	}
	resp, err := transcribe(ctx, settings.OpenRouterAPIKey, audio, name, language)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"text":     strings.TrimSpace(resp.Text),
		"language": resp.Language,
		"words":    transcriptWords(resp),
	})
}

// transcribe asks each of transcribeModels in turn until one answers.
func transcribe(ctx context.Context, key string, audio []byte, filename, language string) (transcriptionResponse, error) {
	var lastErr error
	for _, model := range transcribeModels {
		resp, status, err := transcribeWith(ctx, key, model, audio, filename, language)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		// Only a refusal of this model moves on; a bad key, a rate limit or
		// an outage would fail the same way with the next one.
		if status != http.StatusBadRequest && status != http.StatusNotFound && status != http.StatusUnprocessableEntity {
			break
		}
	}
	return transcriptionResponse{}, lastErr
}

func transcribeWith(ctx context.Context, key, model string, audio []byte, filename, language string) (transcriptionResponse, int, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return transcriptionResponse{}, 0, err
	}
	part.Write(audio)
	w.WriteField("model", model)
	w.WriteField("response_format", "verbose_json")
	w.WriteField("timestamp_granularities[]", "word")
	w.WriteField("timestamp_granularities[]", "segment")
	if language != "" {
		w.WriteField("language", language)
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterTranscribeURL, &body)
	if err != nil {
		return transcriptionResponse{}, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return transcriptionResponse{}, 0, fmt.Errorf("failed to reach OpenRouter: %w", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 10<<20))
	if res.StatusCode != http.StatusOK {
		return transcriptionResponse{}, res.StatusCode, fmt.Errorf("OpenRouter returned %d: %s", res.StatusCode, strings.TrimSpace(string(data)))
	}
	var out transcriptionResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return transcriptionResponse{}, res.StatusCode, fmt.Errorf("invalid transcription from OpenRouter")
	}
	return out, res.StatusCode, nil
}
