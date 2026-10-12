package handlers

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestValidRedirectURI(t *testing.T) {
	cases := map[string]bool{
		"https://claude.ai/api/mcp/auth_callback":               true,
		"https://chatgpt.com/connector_platform_oauth_redirect": true,
		"http://localhost:33418/callback":                       true,
		"http://127.0.0.1:5000/cb":                              true,
		"http://[::1]:5000/cb":                                  true,
		"cursor://anysphere.cursor-retrieval/oauth/callback":    true,
		"http://example.com/cb":                                 false,
		"https://claude.ai/cb#frag":                             false,
		"javascript:alert(1)":                                   false,
		"data:text/html,hi":                                     false,
		"/relative/path":                                        false,
		"https:///nohost":                                       false,
	}
	for uri, want := range cases {
		if got := validRedirectURI(uri); got != want {
			t.Errorf("validRedirectURI(%q) = %v, want %v", uri, got, want)
		}
	}
}

func TestRedirectURIMatches(t *testing.T) {
	cases := []struct {
		registered, requested string
		want                  bool
	}{
		{"https://claude.ai/api/mcp/auth_callback", "https://claude.ai/api/mcp/auth_callback", true},
		{"https://claude.ai/api/mcp/auth_callback", "https://claude.ai/api/mcp/auth_callback/x", false},
		{"https://claude.ai/cb", "https://evil.example/cb", false},
		// Loopback redirects may pick any port at runtime.
		{"http://localhost:1234/callback", "http://localhost:5678/callback", true},
		{"http://127.0.0.1/callback", "http://127.0.0.1:5678/callback", true},
		{"http://localhost:1234/callback", "http://localhost:5678/other", false},
		{"http://localhost:1234/callback", "http://127.0.0.1:1234/callback", false},
		{"https://localhost:1234/callback", "https://localhost:5678/callback", false},
	}
	for _, tc := range cases {
		if got := redirectURIMatches(tc.registered, tc.requested); got != tc.want {
			t.Errorf("redirectURIMatches(%q, %q) = %v, want %v", tc.registered, tc.requested, got, tc.want)
		}
	}
}

func TestNormalizeScope(t *testing.T) {
	cases := map[string]string{
		"":                             "mcp",
		"openid":                       "mcp openid",
		"email openid unknown profile": "mcp openid profile email",
		"admin":                        "mcp",
		"offline_access mcp":           "mcp offline_access",
	}
	for in, want := range cases {
		if got := normalizeScope(in); got != want {
			t.Errorf("normalizeScope(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVerifyPKCE(t *testing.T) {
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	if !verifyPKCE(verifier, challenge) {
		t.Fatal("valid verifier rejected")
	}
	if verifyPKCE(strings.Repeat("b", 43), challenge) {
		t.Fatal("wrong verifier accepted")
	}
	short := "abc"
	s := sha256.Sum256([]byte(short))
	if verifyPKCE(short, base64.RawURLEncoding.EncodeToString(s[:])) {
		t.Fatal("verifier shorter than 43 characters accepted")
	}
}

func TestClientRedirectKeepsQueryAndAddsIssuer(t *testing.T) {
	h := &OAuthHandler{Issuer: "https://pod.example"}
	got := h.clientRedirect("https://app.example/cb?x=1", map[string]string{"code": "abc", "state": "s t", "error": ""})
	want := "https://app.example/cb?code=abc&iss=https%3A%2F%2Fpod.example&state=s+t&x=1"
	if got != want {
		t.Fatalf("clientRedirect = %q, want %q", got, want)
	}
}

func TestNegotiateProtocolVersion(t *testing.T) {
	cases := map[string]string{
		`{"protocolVersion":"2025-06-18"}`: "2025-06-18",
		`{"protocolVersion":"2024-11-05"}`: "2024-11-05",
		`{"protocolVersion":"2099-01-01"}`: mcpProtocolVersions[0],
		``:                                 mcpProtocolVersions[0],
	}
	for in, want := range cases {
		if got := negotiateProtocolVersion(json.RawMessage(in)); got != want {
			t.Errorf("negotiateProtocolVersion(%s) = %q, want %q", in, got, want)
		}
	}
}
