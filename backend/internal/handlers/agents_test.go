package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"socialmedia/internal/models"
)

func TestNormalizeAgentSchedule(t *testing.T) {
	s, err := normalizeAgentSchedule(models.AgentSchedule{Kind: "daily", Times: []string{"18:00", "9:05", "18:00"}, Weekdays: []int{4, 0, 4}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.Times, ",") != "09:05,18:00" || len(s.Weekdays) != 2 || s.Weekdays[0] != 0 || s.Timezone != defaultAgentTimezone {
		t.Errorf("normalized = %+v", s)
	}
	all, _ := normalizeAgentSchedule(models.AgentSchedule{Kind: "daily", Times: []string{"10:00"}, Weekdays: []int{0, 1, 2, 3, 4, 5, 6}})
	if all.Weekdays != nil {
		t.Errorf("all seven days should mean every day, got %v", all.Weekdays)
	}
	for _, bad := range []models.AgentSchedule{
		{Kind: "interval", IntervalMinutes: 5},
		{Kind: "hourly", Minute: 60},
		{Kind: "daily"},
		{Kind: "daily", Times: []string{"25:00"}},
		{Kind: "daily", Times: []string{"10:00"}, Weekdays: []int{7}},
		{Kind: "manual", Timezone: "Mars/Olympus"},
		{Kind: "sometimes"},
	} {
		if _, err := normalizeAgentSchedule(bad); err == nil {
			t.Errorf("%+v should be refused", bad)
		}
	}
}

func TestNextAgentRun(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	// Wednesday 15 October 2025, 10:20 in Berlin.
	now := time.Date(2025, 10, 15, 10, 20, 0, 0, berlin)
	at := func(s models.AgentSchedule) string {
		s.Timezone = "Europe/Berlin"
		n := nextAgentRun(s, now)
		if n == nil {
			return "nil"
		}
		return n.In(berlin).Format("Mon 2006-01-02 15:04")
	}
	cases := []struct {
		s    models.AgentSchedule
		want string
	}{
		{models.AgentSchedule{Kind: "manual"}, "nil"},
		{models.AgentSchedule{Kind: "interval", IntervalMinutes: 90}, "Wed 2025-10-15 11:50"},
		{models.AgentSchedule{Kind: "hourly", Minute: 30}, "Wed 2025-10-15 10:30"},
		{models.AgentSchedule{Kind: "hourly", Minute: 20}, "Wed 2025-10-15 11:20"},
		{models.AgentSchedule{Kind: "daily", Times: []string{"08:00", "18:00"}}, "Wed 2025-10-15 18:00"},
		{models.AgentSchedule{Kind: "daily", Times: []string{"08:00"}}, "Thu 2025-10-16 08:00"},
		// Weekdays 0 = Monday: Monday and Friday.
		{models.AgentSchedule{Kind: "daily", Times: []string{"07:30"}, Weekdays: []int{0, 4}}, "Fri 2025-10-17 07:30"},
		{models.AgentSchedule{Kind: "daily", Times: []string{"07:30"}, Weekdays: []int{2}}, "Wed 2025-10-22 07:30"},
	}
	for _, c := range cases {
		if got := at(c.s); got != c.want {
			t.Errorf("%+v: next = %s, want %s", c.s, got, c.want)
		}
	}

	// Across the end of summer time the wall-clock time stays put.
	before := time.Date(2025, 10, 25, 12, 0, 0, 0, berlin)
	n := nextAgentRun(models.AgentSchedule{Kind: "daily", Times: []string{"09:00"}, Timezone: "Europe/Berlin"}, before)
	if got := n.In(berlin).Format("2006-01-02 15:04"); got != "2025-10-26 09:00" {
		t.Errorf("DST: next = %s", got)
	}
}

func TestAgentOrigin(t *testing.T) {
	cases := map[string]string{
		"News Scout":     "agent-news-scout",
		"  Spiel 2025!!": "agent-spiel-2025",
		"???":            "agent",
		"A very long agent name that goes on and on": "agent-a-very-long-agent-name-tha",
	}
	for in, want := range cases {
		got := agentOrigin(in)
		if got != want {
			t.Errorf("agentOrigin(%q) = %q, want %q", in, got, want)
		}
		if !originRe.MatchString(got) {
			t.Errorf("agentOrigin(%q) = %q is not a valid origin", in, got)
		}
	}
}

// Providers behind OpenRouter reject array schemas without items, so every
// tool an agent sees must spell them out.
func TestAgentToolSchemas(t *testing.T) {
	h := &AgentHandler{MCP: &MCPHandler{}}
	var check func(tool, path string, schema map[string]any)
	check = func(tool, path string, schema map[string]any) {
		if schema["type"] == "array" {
			if _, ok := schema["items"]; !ok {
				t.Errorf("%s: %s is an array without items", tool, path)
			}
		}
		for k, v := range schema {
			switch sub := v.(type) {
			case map[string]any:
				check(tool, path+"."+k, sub)
			}
		}
	}
	names := map[string]bool{}
	for _, tool := range h.agentTools() {
		fn := tool["function"].(map[string]any)
		name := fn["name"].(string)
		if names[name] {
			t.Errorf("tool %s is listed twice", name)
		}
		names[name] = true
		check(name, "parameters", fn["parameters"].(map[string]any))
	}
	for _, want := range []string{"create_post", "get_open_slots", "web_search", "fetch_url"} {
		if !names[want] {
			t.Errorf("agents lack the %s tool", want)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body any) *http.Response {
	raw, _ := json.Marshal(body)
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(raw)), Header: http.Header{}}
}

func TestAgentConversation(t *testing.T) {
	var requests []map[string]any
	orig := agentHTTPClient
	defer func() { agentHTTPClient = orig }()
	agentHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("missing key header")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		switch len(requests) {
		case 1:
			return jsonResponse(200, map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{
					"role": "assistant", "content": "Looking for slots.",
					"tool_calls": []any{
						map[string]any{"id": "c1", "type": "function", "function": map[string]any{"name": "create_post", "arguments": `{"content":"Hi"}`}},
						map[string]any{"id": "c2", "type": "function", "function": map[string]any{"name": "get_image", "arguments": `{bad`}},
					},
				}}},
				"usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 20, "cost": 0.01},
			}), nil
		case 2:
			// The image in the conversation is refused once.
			return jsonResponse(404, map[string]any{"error": map[string]any{"message": "No endpoints found that support image input"}}), nil
		default:
			return jsonResponse(200, map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "Created one draft."}}},
			}), nil
		}
	})}

	var calls []map[string]any
	var steps []models.AgentRunStep
	var usage agentUsage
	conv := &agentConversation{
		key:      "sk-test",
		model:    "test/model",
		messages: chatMessages("system", "Find news."),
		maxSteps: 5,
		origin:   "agent-scout",
		callTool: func(name string, args map[string]any) (any, bool) {
			calls = append(calls, args)
			return &mcpRichResult{Result: map[string]any{"id": "p1"}, Images: []mcpImage{{Data: []byte{1, 2}, MimeType: "image/png"}}}, false
		},
		onStep:  func(s models.AgentRunStep) { steps = append(steps, s) },
		onUsage: func(u agentUsage) { usage.Cost += u.Cost },
	}
	status, summary, errMsg := conv.run(t.Context())
	if status != models.AgentRunOK || summary != "Created one draft." || errMsg != "" {
		t.Fatalf("run = %q %q %q", status, summary, errMsg)
	}
	if len(calls) != 1 || calls[0]["origin"] != "agent-scout" {
		t.Errorf("tool calls = %v, want one create_post with the agent origin", calls)
	}
	if len(steps) != 3 || steps[0].Kind != "message" || steps[2].Tool != "get_image" || !steps[2].IsError {
		t.Errorf("steps = %+v", steps)
	}
	if usage.Cost != 0.01 {
		t.Errorf("usage cost = %v", usage.Cost)
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(requests))
	}
	// The second request carried the image, the retry did not.
	if !strings.Contains(mustJSON(requests[1]), "image_url") || strings.Contains(mustJSON(requests[2]), "image_url") {
		t.Errorf("the image fallback did not drop the image")
	}
	msgs := requests[2]["messages"].([]any)
	var roles []string
	for _, m := range msgs {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	if got := strings.Join(roles, ","); got != "system,user,assistant,tool,tool,user" {
		t.Errorf("roles = %s", got)
	}
}

func TestAgentConversationStepLimit(t *testing.T) {
	orig := agentHTTPClient
	defer func() { agentHTTPClient = orig }()
	agentHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role":       "assistant",
				"tool_calls": []any{map[string]any{"id": "c", "type": "function", "function": map[string]any{"name": "list_posts", "arguments": "{}"}}},
			}}},
		}), nil
	})}
	conv := &agentConversation{
		messages: chatMessages("s", "u"), maxSteps: 2,
		callTool: func(string, map[string]any) (any, bool) { return map[string]any{}, false },
		onStep:   func(models.AgentRunStep) {}, onUsage: func(agentUsage) {},
	}
	if status, _, _ := conv.run(t.Context()); status != models.AgentRunLimit {
		t.Errorf("status = %s, want limit", status)
	}
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestAgentSystemPromptHasNoEmDash(t *testing.T) {
	p := agentSystemPrompt(models.Agent{Name: "Scout"}, "Be brief.", time.Now())
	if strings.ContainsRune(p, '—') || strings.ContainsRune(webSearchSystemPrompt, '—') {
		t.Error("built-in agent prompts must not contain an em dash")
	}
	if !strings.HasSuffix(p, "Be brief.") || !strings.Contains(p, "agent-scout") {
		t.Errorf("prompt lacks the global prompt or origin: %s", p)
	}
}

func TestHTMLToText(t *testing.T) {
	page := `<html><head><title>T</title><style>p{}</style><script>var x = 1;</script></head>
<body><h1>Big &amp; news</h1><p>First   para.</p><!-- hidden --><p>Second</p></body></html>`
	got := htmlToText(page)
	for _, bad := range []string{"var x", "p{}", "hidden", "<"} {
		if strings.Contains(got, bad) {
			t.Errorf("text keeps %q: %q", bad, got)
		}
	}
	if !strings.Contains(got, "Big & news") || !strings.Contains(got, "First para.\n\nSecond") {
		t.Errorf("text = %q", got)
	}
}

func TestParseAgentModels(t *testing.T) {
	body := `{"data":[
		{"id":"a/x","name":"X","context_length":1000,"pricing":{"prompt":"0.000003","completion":"0.000015"},"architecture":{"input_modalities":["text","image"]},"supported_parameters":["tools","temperature"]},
		{"id":"b/y","name":"Y","pricing":{"prompt":"0","completion":"0"},"supported_parameters":["temperature"]}
	]}`
	list, err := parseAgentModels([]byte(body))
	if err != nil || len(list) != 1 {
		t.Fatalf("models = %v, %v", list, err)
	}
	m := list[0]
	if m.ID != "a/x" || !m.Images || m.PromptPrice < 2.99 || m.PromptPrice > 3.01 || m.CompletionPrice < 14.99 {
		t.Errorf("model = %+v", m)
	}
}

func TestTruncateText(t *testing.T) {
	if got := truncateText("äöü", 3); !strings.HasPrefix(got, "ä ") {
		t.Errorf("truncateText cut inside a rune: %q", got)
	}
	if truncateText("short", 10) != "short" {
		t.Error("short text changed")
	}
}
