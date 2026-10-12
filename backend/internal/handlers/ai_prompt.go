package handlers

import "strings"

// noDashRule is appended to the system prompt of every AI request. The em
// dash is a tell-tale sign of machine-written text, so it is banned outright,
// whatever prompt an admin or team configured.
const noDashRule = `ABSOLUTE RULE, NO EXCEPTIONS: never use the em dash character (U+2014) anywhere in your reply.
Do not replace it with an en dash (U+2013), a double hyphen (--) or a spaced hyphen ( - ) either.
Where you would use a dash, write a comma, a colon, a period or parentheses instead, or split the sentence in two.
Check your reply before sending it: if it contains an em dash, rewrite it.`

// withNoDashRule appends noDashRule to a system prompt.
func withNoDashRule(system string) string {
	system = strings.TrimSpace(system)
	if system == "" {
		return noDashRule
	}
	return system + "\n\n" + noDashRule
}

// chatMessages builds the messages of a chat completion request: the system
// prompt, carrying noDashRule, followed by the user content (a string, or a
// multimodal content array). Every AI request goes through it so the rule
// cannot be forgotten.
func chatMessages(system string, user any) []map[string]any {
	return []map[string]any{
		{"role": "system", "content": withNoDashRule(system)},
		{"role": "user", "content": user},
	}
}
