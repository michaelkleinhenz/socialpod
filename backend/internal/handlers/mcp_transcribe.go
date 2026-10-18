package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// Speech tools for agents: the transcription behind the image editor's
// auto-captions, for a reel's clip or sound, so an agent can write a reel's
// caption from what is said in it.

func transcribeToolDefinitions() []mcpTool {
	return []mcpTool{
		{
			Name: "transcribe",
			Description: "Transcribe the speech in a video or sound (Whisper via OpenRouter, like the video editor's auto-captions). " +
				"Give the file by url (an upload such as a reel's video, or a public link) or mediaId (a library video or sound, see list_media). " +
				"Returns the full text, the detected language and phrases with start/end times in seconds (cut at pauses and sentence ends), " +
				"and every word with its times when words is true. Use it to write a reel's caption or hook from what is said, or to quote it. " +
				"Files up to 25 MB (mp4, mov, webm, mp3, m4a, aac, wav, ogg).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":      map[string]any{"type": "string", "description": "Video or sound: an upload URL (/api/uploads/...) or a public URL"},
					"mediaId":  map[string]any{"type": "string", "description": "A media library video or sound"},
					"language": map[string]any{"type": "string", "description": "Spoken language as ISO 639-1 (e.g. de, en); detected when omitted"},
					"words":    map[string]any{"type": "boolean", "description": "Also return every word with its start/end time"},
				},
			},
		},
	}
}

// transcribeExtensions maps what a file is to the name Whisper decodes it by;
// it accepts mp4/m4a for the QuickTime and raw AAC containers too.
var transcribeExtensions = map[string]string{
	".mp4": ".mp4", ".mov": ".mp4", ".m4v": ".mp4", ".webm": ".webm",
	".mp3": ".mp3", ".m4a": ".m4a", ".aac": ".m4a", ".wav": ".wav", ".ogg": ".ogg", ".oga": ".ogg",
}

// transcribeFilename names the file for the transcription request from its
// URL's extension, or from its sniffed type when that tells nothing.
func transcribeFilename(src string, data []byte) (string, error) {
	p := src
	if u, err := url.Parse(src); err == nil {
		p = u.Path
	}
	if ext, ok := transcribeExtensions[strings.ToLower(path.Ext(p))]; ok {
		return "audio" + ext, nil
	}
	switch ct := http.DetectContentType(data); {
	case ct == "video/mp4":
		return "audio.mp4", nil
	case ct == "video/webm":
		return "audio.webm", nil
	case ct == "audio/mpeg":
		return "audio.mp3", nil
	case ct == "audio/wave":
		return "audio.wav", nil
	case ct == "application/ogg":
		return "audio.ogg", nil
	default:
		return "", fmt.Errorf("not a video or sound file (%s)", ct)
	}
}

// TranscriptPhrase is a run of words said together.
type TranscriptPhrase struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// transcriptPhrases groups words into phrases: a new one starts after a pause,
// a sentence end, or once a phrase is long enough to read at a glance.
func transcriptPhrases(words []TranscriptWord) []TranscriptPhrase {
	const pause, maxWords, maxSeconds = 0.6, 10, 5.0
	out := []TranscriptPhrase{}
	var cur []string
	var start, end float64
	flush := func() {
		if len(cur) > 0 {
			out = append(out, TranscriptPhrase{Text: strings.Join(cur, " "), Start: start, End: end})
			cur = nil
		}
	}
	for _, w := range words {
		if len(cur) > 0 && (w.Start-end > pause || len(cur) >= maxWords || w.End-start > maxSeconds) {
			flush()
		}
		if len(cur) == 0 {
			start = w.Start
		}
		cur = append(cur, w.Word)
		end = w.End
		if strings.ContainsAny(w.Word[len(w.Word)-1:], ".!?") {
			flush()
		}
	}
	flush()
	return out
}

func (h *MCPHandler) toolTranscribe(c *gin.Context, args map[string]any) (any, bool) {
	language := strings.ToLower(strings.TrimSpace(strArg(args, "language")))
	if language != "" && !languageCodeRe.MatchString(language) {
		return map[string]string{"error": "language must be an ISO 639-1 code such as de or en"}, true
	}
	if strArg(args, "url") == "" && strArg(args, "mediaId") == "" {
		return map[string]string{"error": "give url or mediaId"}, true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	data, src, err := h.loadImageBytes(ctx, c, map[string]any{"url": args["url"], "mediaId": args["mediaId"]})
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	// A remote download stops at the limit, so reaching it means too large.
	if len(data) >= maxTranscribeBytes {
		return map[string]string{"error": "the file is 25 MB or larger, too large to transcribe"}, true
	}
	name, err := transcribeFilename(src, data)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}

	var settings models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil || settings.OpenRouterAPIKey == "" {
		return map[string]string{"error": "OpenRouter is not configured"}, true
	}
	resp, err := transcribe(ctx, settings.OpenRouterAPIKey, data, name, language)
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	words := transcriptWords(resp)
	out := map[string]any{
		"url":      src,
		"text":     strings.TrimSpace(resp.Text),
		"language": resp.Language,
		"phrases":  transcriptPhrases(words),
	}
	if boolArg(args, "words") {
		out["words"] = words
	}
	return out, false
}
