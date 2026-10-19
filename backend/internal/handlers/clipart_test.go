package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// clipartFixture is an icon set with aliases that flip, turn and resize;
// the expected bodies and view boxes are what @iconify/utils' getIconData
// and iconToSVG make of it.
const clipartFixture = `{"prefix": "x", "width": 24, "height": 20, "icons": {"a": {"body": "<path d=\"M1 2h3\"/>"}, "b": {"body": "<defs><linearGradient id=\"g\"/></defs><path fill=\"url(#g)\" d=\"M0 0h1\"/>", "left": 2, "top": 1, "width": 30, "height": 20}}, "aliases": {"r1": {"parent": "a", "rotate": 1}, "h": {"parent": "a", "hFlip": true}, "hv": {"parent": "a", "hFlip": true, "vFlip": true}, "v": {"parent": "b", "vFlip": true}, "r3": {"parent": "b", "rotate": 3}, "chain": {"parent": "h", "rotate": 2, "width": 10}}}`

func TestIconifyResolveMatchesIconifyUtils(t *testing.T) {
	var set iconifyJSON
	if err := json.Unmarshal([]byte(clipartFixture), &set); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		body    string
		viewBox [4]float64
	}{
		{"a", "<path d=\"M1 2h3\"/>", [4]float64{0, 0, 24, 20}},
		{"b", "<defs><linearGradient id=\"g\"/></defs><path fill=\"url(#g)\" d=\"M0 0h1\"/>", [4]float64{2, 1, 30, 20}},
		{"r1", "<g transform=\"rotate(90 10 10)\"><path d=\"M1 2h3\"/></g>", [4]float64{0, 0, 20, 24}},
		{"h", "<g transform=\"translate(24 0) scale(-1 1)\"><path d=\"M1 2h3\"/></g>", [4]float64{0, 0, 24, 20}},
		{"hv", "<g transform=\"rotate(180 12 10)\"><path d=\"M1 2h3\"/></g>", [4]float64{0, 0, 24, 20}},
		{"v", "<defs><linearGradient id=\"g\"/></defs><g transform=\"translate(-2 21) scale(1 -1)\"><path fill=\"url(#g)\" d=\"M0 0h1\"/></g>", [4]float64{0, 0, 30, 20}},
		{"r3", "<defs><linearGradient id=\"g\"/></defs><g transform=\"rotate(-90 17 17)\"><path fill=\"url(#g)\" d=\"M0 0h1\"/></g>", [4]float64{1, 2, 20, 30}},
		{"chain", "<g transform=\"rotate(180 5 10) translate(10 0) scale(-1 1)\"><path d=\"M1 2h3\"/></g>", [4]float64{0, 0, 10, 20}},
	} {
		icon, ok := set.resolve(tc.name)
		if !ok {
			t.Fatalf("%s: not resolved", tc.name)
		}
		if icon.Body != tc.body {
			t.Errorf("%s: body\n got %s\nwant %s", tc.name, icon.Body, tc.body)
		}
		if got := [4]float64{icon.Left, icon.Top, icon.Width, icon.Height}; got != tc.viewBox {
			t.Errorf("%s: view box %v, want %v", tc.name, got, tc.viewBox)
		}
	}
	if _, ok := set.resolve("missing"); ok {
		t.Error("an unknown icon resolved")
	}
}

func TestClipartSearchKeepsFreeSetsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var searches int
	iconify := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			searches++
			if r.URL.Query().Get("query") != "arrow" || !strings.Contains(r.URL.Query().Get("prefixes"), "tabler") {
				t.Errorf("unexpected search %s", r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"icons": []string{"tabler:arrow-right", "solar:arrow-up", "mdi:arrow-left", "tabler:bad_name", "noto:right-arrow"},
				"collections": map[string]any{
					"tabler": map[string]any{"name": "Tabler Icons", "license": map[string]any{"spdx": "MIT"}},
					"solar":  map[string]any{"name": "Solar", "license": map[string]any{"spdx": "CC-BY-4.0"}},
					// A set that changed to a licence asking for credit is left out.
					"mdi":  map[string]any{"name": "Material Design Icons", "license": map[string]any{"spdx": "CC-BY-4.0"}},
					"noto": map[string]any{"name": "Noto Emoji", "license": map[string]any{"spdx": "Apache-2.0"}},
				},
			})
		case "/tabler.json":
			if got := r.URL.Query().Get("icons"); got != "arrow-right" {
				t.Errorf("tabler icons %q", got)
			}
			w.Write([]byte(`{"prefix":"tabler","width":24,"height":24,"icons":{"arrow-right":{"body":"<path stroke=\"currentColor\" d=\"M5 12h14\"/>"}}}`))
		case "/noto.json":
			w.Write([]byte(`{"prefix":"noto","width":128,"height":128,"icons":{"left-arrow":{"body":"<path fill=\"#40C0E7\" d=\"M0 0h1\"/>"}},"aliases":{"right-arrow":{"parent":"left-arrow","hFlip":true}}}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer iconify.Close()

	h := NewClipartHandler(iconify.URL)
	search := func() clipartSearchResult {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/clipart/search?q="+url.QueryEscape(" arrow "), nil)
		h.Search(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var res clipartSearchResult
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := search()
	if len(res.Icons) != 2 || res.Icons[0].ID != "tabler:arrow-right" || res.Icons[1].ID != "noto:right-arrow" {
		t.Fatalf("icons %+v", res.Icons)
	}
	if !res.Icons[0].Recolorable || res.Icons[1].Recolorable {
		t.Errorf("recolorable: %v %v", res.Icons[0].Recolorable, res.Icons[1].Recolorable)
	}
	if res.Icons[0].Width != 24 || res.Icons[1].Width != 128 {
		t.Errorf("sizes %v %v", res.Icons[0].Width, res.Icons[1].Width)
	}
	if !strings.HasPrefix(res.Icons[1].Body, `<g transform="translate(128 0) scale(-1 1)">`) {
		t.Errorf("alias body %s", res.Icons[1].Body)
	}
	if _, ok := res.Sets["mdi"]; ok || res.Sets["tabler"].License != "MIT" {
		t.Errorf("sets %+v", res.Sets)
	}
	search()
	if searches != 1 {
		t.Errorf("searched Iconify %d times, want the second answer from the cache", searches)
	}
}

func TestClipartSearchNeedsQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/clipart/search?q=", nil)
	NewClipartHandler("http://127.0.0.1:1").Search(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d", w.Code)
	}
}
