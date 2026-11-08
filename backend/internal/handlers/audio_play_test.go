package handlers

import (
	"strings"
	"testing"
)

func TestNormalizeAudioPlay(t *testing.T) {
	in := "```\nSzene: Ein Spieltisch am Abend.\n\nMichael (männlich)\nJutta (weiblich)\n\n**Michael:** Hast du die Regeln gelesen?\nJUTTA (lachend): Natürlich nicht.\n- jutta: (flüsternd) Wer würfelt?\nErzähler: Stille.\nMichael:\n```"
	want := "Michael: Hast du die Regeln gelesen?\nJutta: (lachend) Natürlich nicht.\nJutta: (flüsternd) Wer würfelt?"
	if got := normalizeAudioPlay(in); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestNormalizeAudioPlayKeepsTextWithoutDialogue(t *testing.T) {
	in := "  Eine Szene über \"Wingspan\".  "
	if got := normalizeAudioPlay(in); got != strings.TrimSpace(in) {
		t.Fatalf("got %q", got)
	}
}

func TestCapturePromptCarriesAudioPlayFormat(t *testing.T) {
	for _, team := range []string{"", "Custom team prompt."} {
		if !strings.Contains(buildSystemPrompt(team, ""), audioPlayFormatRule) {
			t.Fatalf("prompt for team %q lacks the audio play format rule", team)
		}
	}
	if strings.Contains(audioPlaySystemPrompt+audioPlayFormatRule, "—") {
		t.Fatal("em dash in audio play prompt")
	}
}
