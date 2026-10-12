package handlers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestChatMessagesCarriesNoDashRule(t *testing.T) {
	for _, system := range []string{"", "You are a copywriter."} {
		msgs := chatMessages(system, "hello")
		if len(msgs) != 2 || msgs[0]["role"] != "system" || msgs[1]["role"] != "user" {
			t.Fatalf("unexpected messages: %v", msgs)
		}
		content, _ := msgs[0]["content"].(string)
		if !strings.HasPrefix(content, system) || !strings.HasSuffix(content, noDashRule) {
			t.Errorf("system prompt %q lacks the no-dash rule: %q", system, content)
		}
	}
}

// Every chat completion request must build its messages with chatMessages,
// so none can go out without the no-dash rule.
var chatMessagesUse = regexp.MustCompile(`"messages":\s+chatMessages\(`)

func TestEveryAIRequestUsesChatMessages(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(src)
		calls := strings.Count(s, "/chat/completions")
		built := len(chatMessagesUse.FindAllString(s, -1))
		if calls != built {
			t.Errorf("%s: %d chat completion requests but %d built with chatMessages", f, calls, built)
		}
	}
}
