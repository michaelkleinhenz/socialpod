package handlers

import (
	"regexp"
	"strings"
)

// The review episode's "scene" is a short audio play. It is imported into
// WavePod, whose plain-text format is one line of dialogue per row,
// "NAME: what they say", with an optional direction in brackets after the
// name: "MICHAEL: (whispering) Hello?". Nothing else may be in it: no cast
// list, no speaker genders, no stage directions on lines of their own, no
// blank lines. The roles are always Michael and Jutta.

// audioPlayFormatRule is appended to every prompt that writes a scene: the
// capture prompt (the team's own override included) and the Episode form's
// "Generate with AI", whatever scene prompt template the team configured.
const audioPlayFormatRule = `The "scene" is a short audio play that is imported into WavePod, so it must follow
WavePod's plain-text format exactly:
- One line of dialogue per row, in the form NAME: what they say
- A direction can go in brackets right after the colon, as in Michael: (whispering) Hello?
- The only two roles are Michael and Jutta. Always use exactly these names, spelled
  Michael and Jutta, and no other characters, narrator or sound effect lines.
- Write nothing but these dialogue lines: no title, no cast list, no lines giving a
  speaker's sex, gender, age or voice, no scene descriptions or stage directions on
  lines of their own, no blank lines, no markdown, no bold names, no numbering.`

// audioPlaySystemPrompt is the system prompt for "Generate with AI" on the
// Episode form's Scene field. The team's scene prompt template and the text
// typed into the field arrive as the user message.
const audioPlaySystemPrompt = `You write short, humorous audio play scenes for a board game review podcast,
spoken by its two hosts, Michael and Jutta. The user gives you a prompt and notes about
the game and the scene. Reply with ONLY the scene, no commentary, no quotes around it.

` + audioPlayFormatRule

var (
	audioPlayDecor   = strings.NewReplacer("**", "", "__", "")
	audioPlayBullets = regexp.MustCompile(`^(?:[-*•]\s+|\d+[.)]\s+)`)
)

// normalizeAudioPlay turns a model's scene into WavePod's format: it keeps
// only the lines spoken by Michael or Jutta ("NAME: text"), dropping code
// fences, blank lines, cast lists, narrator and description lines; strips
// markdown emphasis and list markers off the name; spells the names the same
// way every time; and moves a direction a model put between the name and the
// colon ("Jutta (laughing): ...") after it. A text with no such line at all is
// returned trimmed but otherwise as it was, so nothing is thrown away.
func normalizeAudioPlay(s string) string {
	var out []string
	for _, raw := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		line := audioPlayBullets.ReplaceAllString(audioPlayDecor.Replace(strings.TrimSpace(raw)), "")
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		name, text := strings.TrimSpace(line[:colon]), strings.TrimSpace(line[colon+1:])
		direction := ""
		if open := strings.Index(name, "("); open > 0 && strings.HasSuffix(name, ")") {
			direction = strings.TrimSpace(name[open:])
			name = strings.TrimSpace(name[:open])
		}
		role := canonicalRole(name)
		if role == "" || text == "" {
			continue
		}
		if direction != "" {
			text = direction + " " + text
		}
		out = append(out, role+": "+text)
	}
	if len(out) == 0 {
		return strings.TrimSpace(s)
	}
	return strings.Join(out, "\n")
}

// canonicalRole spells the two hosts' names the same way every time, so
// WavePod sees two roles and not "MICHAEL" and "Michael". Any other name
// gives "".
func canonicalRole(name string) string {
	switch strings.ToLower(name) {
	case "michael":
		return "Michael"
	case "jutta":
		return "Jutta"
	}
	return ""
}
