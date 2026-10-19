package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"socialmedia/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// OpenClipart (https://openclipart.org/api/tutorial), searched beside Iconify
// once an admin has stored an API token in the settings. Its clipart is in
// the public domain (CC0). The app tokens are JWTs that expire after a day,
// so with the app ID and secret stored as well a new one is fetched from
// apps/getToken when the stored one runs out, and stored in its place.
//
// The search answer's file entries are not documented beyond
// data.files, so they are read loosely: an id, a title, and whatever URLs
// they carry, sorted into a thumbnail, a PNG and an SVG by key and extension.

const openClipartBaseURL = "https://openclipart.org"

// openClipartSettings are the admin's OpenClipart credentials.
type openClipartSettings struct {
	Token     string
	AppID     string
	AppSecret string
}

func (s openClipartSettings) enabled() bool {
	return s.Token != "" || (s.AppID != "" && s.AppSecret != "")
}

type openClipartAuth struct {
	mu    sync.Mutex
	token string // fetched from getToken, while it lasts
	appID string // the app it was fetched for
}

// openClipartItem is one file of a search answer: a preview to show and the
// file to import.
type openClipartItem struct {
	ID         string
	Title      string
	PreviewURL string
	FileURL    string
}

// jwtExpiry reads a JWT's exp claim; zero when the token is not a JWT or has
// none, which is taken as not expiring.
func jwtExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp float64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(claims.Exp), 0)
}

// tokenValid is true for a token that will last a few more minutes.
func tokenValid(token string) bool {
	if token == "" {
		return false
	}
	exp := jwtExpiry(token)
	return exp.IsZero() || time.Until(exp) > 5*time.Minute
}

func (h *ClipartHandler) loadOpenClipartSettings(ctx context.Context) (openClipartSettings, error) {
	if h.OpenClipartSettings != nil {
		return h.OpenClipartSettings(ctx)
	}
	if h.DB == nil {
		return openClipartSettings{}, nil
	}
	var s models.AppSettings
	if err := h.DB.Settings().FindOne(ctx, bson.M{}).Decode(&s); err != nil {
		return openClipartSettings{}, nil
	}
	return openClipartSettings{Token: s.OpenClipartAPIKey, AppID: s.OpenClipartAppID, AppSecret: s.OpenClipartAppSecret}, nil
}

// openClipartToken is the token to search with, fetching a new one when the
// stored one has run out and the app credentials are there. "" means
// OpenClipart is not set up.
func (h *ClipartHandler) openClipartToken(ctx context.Context) (string, error) {
	s, err := h.loadOpenClipartSettings(ctx)
	if err != nil || !s.enabled() {
		return "", err
	}
	if tokenValid(s.Token) {
		return s.Token, nil
	}
	if s.AppID == "" || s.AppSecret == "" {
		return "", errors.New("the OpenClipart token has expired; enter a new one, or the app ID and secret, in the admin settings")
	}
	h.ocAuth.mu.Lock()
	defer h.ocAuth.mu.Unlock()
	if h.ocAuth.appID == s.AppID && tokenValid(h.ocAuth.token) {
		return h.ocAuth.token, nil
	}
	token, err := h.fetchOpenClipartToken(ctx, s)
	if err != nil {
		return "", fmt.Errorf("getting an OpenClipart token: %w", err)
	}
	h.ocAuth.token, h.ocAuth.appID = token, s.AppID
	if h.DB != nil && h.OpenClipartSettings == nil {
		h.DB.Settings().UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"openClipartApiKey": token}})
	}
	return token, nil
}

func (h *ClipartHandler) fetchOpenClipartToken(ctx context.Context, s openClipartSettings) (string, error) {
	body, _ := json.Marshal(map[string]string{"appid": s.AppID, "secret": s.AppSecret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.openClipartURL()+"/api/v2@beta/apps/getToken", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.Token != "" {
		req.Header.Set("x-openclipart-apikey", s.Token)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenClipart answered %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || out.Data.Token == "" {
		return "", errors.New("no token in OpenClipart's answer")
	}
	return out.Data.Token, nil
}

func (h *ClipartHandler) openClipartURL() string {
	if h.OpenClipartURL != "" {
		return strings.TrimRight(h.OpenClipartURL, "/")
	}
	return openClipartBaseURL
}

func (h *ClipartHandler) searchOpenClipart(ctx context.Context, token, q string) ([]ClipartIcon, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.openClipartURL()+"/api/v2@beta/search?q="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-openclipart-apikey", token)
	req.Header.Set("Accept", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errors.New("OpenClipart refused the token; check it in the admin settings")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenClipart answered %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	items := parseOpenClipartSearch(raw, h.openClipartURL())
	icons := make([]ClipartIcon, 0, len(items))
	for _, it := range items {
		icons = append(icons, ClipartIcon{
			ID: "openclipart:" + it.ID, Source: "openclipart", Set: "openclipart", Name: it.Title,
			PreviewURL: it.PreviewURL, FileURL: it.FileURL,
		})
	}
	return icons, nil
}

// parseOpenClipartSearch reads the files of a search answer: data.files in
// API v2, payload in the older JSON search.
func parseOpenClipartSearch(raw []byte, base string) []openClipartItem {
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	list := findOpenClipartList(doc, 0)
	var out []openClipartItem
	seen := map[string]bool{}
	for _, entry := range list {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		it := openClipartEntry(m, base)
		if it.FileURL == "" || it.PreviewURL == "" || seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}
	return out
}

func findOpenClipartList(v any, depth int) []any {
	if depth > 3 {
		return nil
	}
	switch t := v.(type) {
	case []any:
		return t
	case map[string]any:
		for _, k := range []string{"files", "payload", "results", "items", "clipart", "cliparts", "data"} {
			if child, ok := t[k]; ok {
				if list := findOpenClipartList(child, depth+1); list != nil {
					return list
				}
			}
		}
	}
	return nil
}

func openClipartEntry(m map[string]any, base string) openClipartItem {
	var it openClipartItem
	for _, k := range []string{"id", "uuid", "slug"} {
		switch v := m[k].(type) {
		case string:
			if it.ID == "" && v != "" {
				it.ID = v
			}
		case float64:
			if it.ID == "" {
				it.ID = strconv.FormatInt(int64(v), 10)
			}
		}
	}
	for _, k := range []string{"title", "name"} {
		if s, ok := m[k].(string); ok && s != "" {
			it.Title = s
			break
		}
	}
	var svg, png, bigPNG, thumb string
	var walk func(key string, v any, depth int)
	walk = func(key string, v any, depth int) {
		if depth > 4 {
			return
		}
		switch t := v.(type) {
		case map[string]any:
			for k, child := range t {
				walk(key+"."+strings.ToLower(k), child, depth+1)
			}
		case string:
			u := resolveOpenClipartURL(t, base)
			if u == "" {
				return
			}
			p, _ := url.Parse(u)
			ext := strings.ToLower(path.Ext(p.Path))
			switch {
			case ext == ".svg" || (ext == "" && strings.HasSuffix(key, "svg")):
				if svg == "" {
					svg = u
				}
			case strings.Contains(key, "thumb") || strings.Contains(key, "preview") || strings.Contains(key, "small"):
				if thumb == "" && ext != ".svg" {
					thumb = u
				}
			case ext == ".png" || ext == ".jpg" || ext == ".jpeg" || strings.Contains(key, "png"):
				if strings.Contains(key, "full") || strings.Contains(key, "2400") || strings.Contains(key, "large") {
					if bigPNG == "" {
						bigPNG = u
					}
				} else if png == "" {
					png = u
				}
			}
		}
	}
	for k, v := range m {
		walk(strings.ToLower(k), v, 0)
	}
	if bigPNG == "" {
		bigPNG = png
	}
	// The file: a large PNG as it is, else the SVG for the editor to render.
	it.FileURL = bigPNG
	if it.FileURL == "" {
		it.FileURL = svg
	}
	it.PreviewURL = thumb
	if it.PreviewURL == "" {
		it.PreviewURL = png
	}
	if it.PreviewURL == "" {
		it.PreviewURL = bigPNG
	}
	if it.PreviewURL == "" {
		it.PreviewURL = svg
	}
	if it.ID == "" {
		it.ID = it.FileURL
	}
	if it.Title == "" {
		it.Title = "clipart"
	}
	return it
}

// resolveOpenClipartURL makes a URL in an answer absolute, and keeps only
// those on OpenClipart itself.
func resolveOpenClipartURL(s, base string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \n") {
		return ""
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "/") {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	u, err := b.Parse(s)
	if err != nil || !openClipartHost(u.Hostname(), b.Hostname()) {
		return ""
	}
	return u.String()
}

func openClipartHost(host, base string) bool {
	host = strings.ToLower(host)
	return host == base || host == "openclipart.org" || strings.HasSuffix(host, ".openclipart.org")
}

type openClipartImportInput struct {
	URL string `json:"url"`
}

// ImportOpenClipart answers POST /api/clipart/openclipart/import {url}: a
// PNG or JPEG is stored as an upload ({url}); an SVG comes back as markup
// ({svg}) for the editor to render, like Iconify's icons.
func (h *ClipartHandler) ImportOpenClipart(c *gin.Context) {
	var in openClipartImportInput
	if err := c.ShouldBindJSON(&in); err != nil || in.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !openClipartHost(u.Hostname(), hostOf(h.openClipartURL())) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not an OpenClipart file"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	token, err := h.openClipartToken(ctx)
	if err != nil || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OpenClipart is not set up"})
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Header.Set("x-openclipart-apikey", token)
	client := *h.Client
	client.Timeout = 60 * time.Second
	base := hostOf(h.openClipartURL())
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !openClipartHost(r.URL.Hostname(), base) {
			return errors.New("redirected off OpenClipart")
		}
		r.Header.Set("x-openclipart-apikey", token)
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Download failed: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("OpenClipart answered %d", resp.StatusCode)})
		return
	}
	const maxBytes = 15 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Download failed"})
		return
	}
	if len(data) > maxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "That clipart is too large"})
		return
	}
	ct := http.DetectContentType(data)
	head := strings.ToLower(string(data[:min(len(data), 512)]))
	switch {
	case strings.HasPrefix(ct, "image/png"), strings.HasPrefix(ct, "image/jpeg"):
		ext := ".png"
		if strings.HasPrefix(ct, "image/jpeg") {
			ext = ".jpg"
		}
		stored, err := storeUploadBytes(ctx, h.DB, h.UploadDir, fmt.Sprintf("%d%s", time.Now().UnixNano(), ext), ct, data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store the clipart"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"url": stored})
	case strings.Contains(head, "<svg"):
		c.JSON(http.StatusOK, gin.H{"svg": string(data)})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"error": "OpenClipart sent no image"})
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
