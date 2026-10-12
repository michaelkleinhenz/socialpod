package handlers

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Instagram caption checks, the server-side twin of checkCaption in the
// frontend's services/instagramGrowth.ts, so an agent can check a caption
// against the same growth-plan rules the PostEditor shows. Keep the two in
// step.

// CaptionCheck is one rule's verdict: level is ok, warn or missing.
type CaptionCheck struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Level string `json:"level"`
	Hint  string `json:"hint"`
}

var (
	captionHashtagRe = regexp.MustCompile(`(?:^|\s)#[\p{L}\p{N}_]+`)
	hashtagLineRe    = regexp.MustCompile(`^(?:#[\p{L}\p{N}_]+\s*)+$`)
	// Phrases that ask people to pass a post on, in German and English.
	sharePromptRe    = regexp.MustCompile(`(?i)\b(schick|schickt|schicke|teil|teilt|teile|leite|leitet|markier|markiert|markiere|send|share|tag)\w*\b.{0,60}\b(freund|spielerunde|runde|person|jemand|wem|friend|someone|crew|group|partner)`)
	endsWithQuestion = regexp.MustCompile(`\?\s*\S{0,4}$`)
)

func countHashtags(text string) int {
	return len(captionHashtagRe.FindAllString(text, -1))
}

// captionProse is the caption without its trailing hashtag block.
func captionProse(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for len(lines) > 0 {
		last := strings.TrimSpace(lines[len(lines)-1])
		if last == "" || hashtagLineRe.MatchString(last) {
			lines = lines[:len(lines)-1]
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// checkCaption checks an Instagram caption against the growth plan's caption
// rules. blueskyText, when not nil, adds the "not a copy" check; taggedCount
// is the number of tagged accounts (mentions, image tags, collaborators).
func checkCaption(text string, blueskyText *string, taggedCount int) []CaptionCheck {
	trimmed := strings.TrimSpace(text)
	prose := captionProse(trimmed)
	opening := strings.TrimSpace(strings.SplitN(trimmed, "\n", 2)[0])
	n := utf8.RuneCountInString(opening)
	hashtags := countHashtags(trimmed)
	var checks []CaptionCheck

	hookOK := n >= 10 && n <= 90 && !strings.HasPrefix(opening, "#") && !strings.HasPrefix(opening, "@")
	hook := CaptionCheck{ID: "hook", Label: "Hook in the first line"}
	switch {
	case hookOK:
		hook.Level, hook.Hint = "ok", "Short opening line people see before \"more\"."
	case n > 90:
		hook.Level, hook.Hint = "warn", "The first line is long, Instagram cuts it off. Lead with a short, pointed line."
	case opening != "":
		hook.Level, hook.Hint = "warn", "Open with a line that stops the scroll, e.g. \"Der neue Feld: kaufen oder warten?\""
	default:
		hook.Level, hook.Hint = "missing", "Open with a line that stops the scroll, e.g. \"Der neue Feld: kaufen oder warten?\""
	}
	checks = append(checks, hook)

	lastProse := ""
	for _, l := range strings.Split(prose, "\n") {
		if strings.TrimSpace(l) != "" {
			lastProse = strings.TrimSpace(l)
		}
	}
	if endsWithQuestion.MatchString(lastProse) {
		checks = append(checks, CaptionCheck{"question", "Ends with a question", "ok", "Invites comments."})
	} else {
		checks = append(checks, CaptionCheck{"question", "Ends with a question", "missing", "Close with a question to the community before the hashtags."})
	}

	if sharePromptRe.MatchString(prose) {
		checks = append(checks, CaptionCheck{"share", "Share prompt", "ok", "Asks people to pass it on."})
	} else {
		checks = append(checks, CaptionCheck{"share", "Share prompt", "missing", "Ask for a send, e.g. \"Schick das der Person, die immer die Regeln erklärt.\""})
	}

	tags := CaptionCheck{ID: "hashtags", Label: fmt.Sprintf("3–5 hashtags (%d)", hashtags)}
	switch {
	case hashtags > 5:
		tags.Level, tags.Hint = "warn", "Too many, keep 3 to 5 specific ones like #brettspiele #kennerspiel."
	case hashtags < 3:
		tags.Level, tags.Hint = "missing", "Add 3 to 5 specific hashtags, e.g. #brettspiele #brettspielneuheiten #spielessen."
	default:
		tags.Level, tags.Hint = "ok", "Specific, not a long block."
	}
	checks = append(checks, tags)

	if taggedCount > 0 {
		checks = append(checks, CaptionCheck{"tagged", "Publisher tagged", "ok", "Tagged accounts are notified and often share it in their story."})
	} else {
		checks = append(checks, CaptionCheck{"tagged", "Publisher tagged", "missing", "Mention the publisher (and author) with @ so they are tagged."})
	}

	if blueskyText != nil {
		b := strings.TrimSpace(*blueskyText)
		if b != "" && b == trimmed {
			checks = append(checks, CaptionCheck{"mirror", "Not a copy of the Bluesky text", "warn", "Identical to the Bluesky text: write the Instagram caption for Instagram."})
		} else {
			checks = append(checks, CaptionCheck{"mirror", "Not a copy of the Bluesky text", "ok", "Written for Instagram."})
		}
	}
	return checks
}
