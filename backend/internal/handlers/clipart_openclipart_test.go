package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func testJWT(exp time.Time) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"HS256"}`)) + "." + enc([]byte(fmt.Sprintf(`{"id":"app","exp":%d}`, exp.Unix()))) + ".sig"
}

func TestJWTExpiry(t *testing.T) {
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	if got := jwtExpiry(testJWT(exp)); !got.Equal(exp) {
		t.Errorf("exp %v, want %v", got, exp)
	}
	if !jwtExpiry("not-a-jwt").IsZero() || !tokenValid("not-a-jwt") {
		t.Error("a token that is not a JWT should count as not expiring")
	}
	if tokenValid(testJWT(time.Now().Add(time.Minute))) {
		t.Error("a token about to run out counts as valid")
	}
}

func TestParseOpenClipartSearch(t *testing.T) {
	// API v2: data.files, with relative and absolute URLs.
	v2 := `{"success":true,"status":200,"data":{"files":[
		{"id":123,"title":"Red arrow","svg_url":"/download/123/red-arrow.svg","png_thumb":"https://openclipart.org/image/400px/123","png_full_lossy":"https://openclipart.org/image/2400px/123.png"},
		{"id":124,"title":"Only svg","files":{"svg":"https://openclipart.org/download/124/only.svg"}},
		{"id":125,"title":"Elsewhere","png_thumb":"https://evil.example/thumb.png","png_full_lossy":"https://evil.example/full.png"}
	],"total_results":3,"files_count":3}}`
	items := parseOpenClipartSearch([]byte(v2), openClipartBaseURL)
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	if items[0].ID != "123" || items[0].Title != "Red arrow" ||
		items[0].PreviewURL != "https://openclipart.org/image/400px/123" ||
		items[0].FileURL != "https://openclipart.org/image/2400px/123.png" {
		t.Errorf("first %+v", items[0])
	}
	if items[1].FileURL != "https://openclipart.org/download/124/only.svg" || items[1].PreviewURL != items[1].FileURL {
		t.Errorf("second %+v", items[1])
	}

	// The older JSON search: payload[].svg.{url,png_thumb,png_full_lossy}.
	legacy := `{"msg":"success","payload":[{"title":"Dice","id":"9","svg":{"url":"https://openclipart.org/download/9/dice.svg","png_thumb":"https://openclipart.org/image/90px/svg_to_png/9/dice.png","png_full_lossy":"https://openclipart.org/image/800px/svg_to_png/9/dice.png"}}]}`
	items = parseOpenClipartSearch([]byte(legacy), openClipartBaseURL)
	if len(items) != 1 || items[0].PreviewURL != "https://openclipart.org/image/90px/svg_to_png/9/dice.png" ||
		items[0].FileURL != "https://openclipart.org/image/800px/svg_to_png/9/dice.png" {
		t.Errorf("legacy %+v", items)
	}
}

func TestClipartSearchAddsOpenClipartWithRefreshedToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fresh := testJWT(time.Now().Add(24 * time.Hour))
	var getTokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			json.NewEncoder(w).Encode(map[string]any{
				"icons":       []string{"tabler:arrow-right"},
				"collections": map[string]any{"tabler": map[string]any{"name": "Tabler Icons", "license": map[string]any{"spdx": "MIT"}}},
			})
		case "/tabler.json":
			w.Write([]byte(`{"prefix":"tabler","width":24,"height":24,"icons":{"arrow-right":{"body":"<path/>"}}}`))
		case "/api/v2@beta/apps/getToken":
			getTokens++
			var in map[string]string
			json.NewDecoder(r.Body).Decode(&in)
			if in["appid"] != "app1" || in["secret"] != "s3cret" {
				t.Errorf("getToken body %v", in)
			}
			fmt.Fprintf(w, `{"success":true,"status":200,"data":{"auth":true,"token":%q}}`, fresh)
		case "/api/v2@beta/search":
			if r.Header.Get("x-openclipart-apikey") != fresh {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("q") != "arrow" {
				t.Errorf("q %q", r.URL.Query().Get("q"))
			}
			fmt.Fprintf(w, `{"success":true,"data":{"files":[{"id":7,"title":"Arrow","png_thumb":"/image/400px/7","png_full_lossy":"/image/2400px/7.png"}]}}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	h := NewClipartHandler(nil, "", srv.URL)
	h.OpenClipartURL = srv.URL
	h.OpenClipartSettings = func(context.Context) (openClipartSettings, error) {
		// The stored token has expired: a new one comes from getToken.
		return openClipartSettings{Token: testJWT(time.Now().Add(-time.Hour)), AppID: "app1", AppSecret: "s3cret"}, nil
	}
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/clipart/search?q=arrow", nil)
		h.Search(c)
		var res clipartSearchResult
		json.Unmarshal(w.Body.Bytes(), &res)
		if w.Code != http.StatusOK || len(res.Icons) != 2 || res.Errors != nil {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		oc := res.Icons[1]
		if oc.Source != "openclipart" || oc.ID != "openclipart:7" || oc.FileURL != srv.URL+"/image/2400px/7.png" || oc.Body != "" {
			t.Errorf("openclipart result %+v", oc)
		}
		if res.Icons[0].Source != "iconify" || res.Sets["openclipart"].License != "CC0-1.0" {
			t.Errorf("result %+v", res)
		}
	}
	if getTokens != 1 {
		t.Errorf("getToken called %d times, want once (then the fetched token is reused)", getTokens)
	}
}

func TestClipartSearchReportsExpiredOpenClipartToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Write([]byte(`{"icons":[],"collections":{}}`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	h := NewClipartHandler(nil, "", srv.URL)
	h.OpenClipartURL = srv.URL
	h.OpenClipartSettings = func(context.Context) (openClipartSettings, error) {
		return openClipartSettings{Token: testJWT(time.Now().Add(-time.Hour))}, nil
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/clipart/search?q=arrow", nil)
	h.Search(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"openclipart":"the OpenClipart token has expired`) {
		t.Errorf("status %d: %s", w.Code, w.Body.String())
	}
}

func svgImported(w *httptest.ResponseRecorder) bool {
	var out struct{ SVG string }
	return w.Code == http.StatusOK && json.Unmarshal(w.Body.Bytes(), &out) == nil && strings.Contains(out.SVG, "<svg")
}

func TestImportOpenClipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := testJWT(time.Now().Add(time.Hour))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-openclipart-apikey") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`))
	}))
	defer srv.Close()
	h := NewClipartHandler(nil, "", srv.URL)
	h.OpenClipartURL = srv.URL
	h.OpenClipartSettings = func(context.Context) (openClipartSettings, error) { return openClipartSettings{Token: token}, nil }
	post := func(u string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/clipart/openclipart/import", bytes.NewBufferString(fmt.Sprintf(`{"url":%q}`, u)))
		c.Request.Header.Set("Content-Type", "application/json")
		h.ImportOpenClipart(c)
		return w
	}
	if w := post(srv.URL + "/download/1/a.svg"); !svgImported(w) {
		t.Errorf("svg import %d: %s", w.Code, w.Body.String())
	}
	if w := post("https://evil.example/a.png"); w.Code != http.StatusBadRequest {
		t.Errorf("a file off OpenClipart was fetched: %d", w.Code)
	}
}
