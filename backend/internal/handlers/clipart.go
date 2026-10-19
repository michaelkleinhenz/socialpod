package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Clipart for the image editor: arrows, symbols, emoji and the like from
// Iconify (https://iconify.design), which serves the open-source icon sets
// behind one API. Only sets whose licence asks for no attribution in a
// published image are searched, so a post never needs a credit line.
//
// The browser never talks to Iconify: the search goes through here, and the
// answer carries each icon as SVG markup, which the editor recolours, renders
// to a PNG and uploads, so the picture holds a normal upload.

// DefaultIconifyAPIURL is Iconify's public API. ICONIFY_API_URL points the
// search at a self-hosted copy (github.com/iconify/api) instead.
const DefaultIconifyAPIURL = "https://api.iconify.design"

// clipartSets are the icon sets searched, in the order they are listed.
var clipartSets = []string{
	"fluent-emoji-flat", // MIT, colour emoji
	"noto",              // Apache-2.0, colour emoji
	"tabler",            // MIT
	"ph",                // MIT, Phosphor
	"mdi",               // Apache-2.0, Material Design Icons
	"material-symbols",  // Apache-2.0
	"lucide",            // ISC
	"iconoir",           // MIT
	"mingcute",          // Apache-2.0
	"hugeicons",         // MIT
	"carbon",            // Apache-2.0
	"fluent",            // MIT, Fluent UI
}

// clipartLicenses are the licences a set must still carry when Iconify
// reports it, in case one of the sets above changes its licence.
var clipartLicenses = map[string]bool{"MIT": true, "ISC": true, "Apache-2.0": true, "CC0-1.0": true}

var clipartNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const (
	clipartCacheTTL     = time.Hour
	clipartCacheEntries = 500
)

type ClipartHandler struct {
	BaseURL string
	Client  *http.Client

	mu    sync.Mutex
	cache map[string]clipartCacheEntry
}

type clipartCacheEntry struct {
	at  time.Time
	res clipartSearchResult
}

func NewClipartHandler(baseURL string) *ClipartHandler {
	if baseURL == "" {
		baseURL = DefaultIconifyAPIURL
	}
	return &ClipartHandler{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  &http.Client{Timeout: 15 * time.Second},
		cache:   map[string]clipartCacheEntry{},
	}
}

// ClipartIcon is one search result: the SVG's inner markup and its view box,
// already with the icon's own flips and turns applied.
type ClipartIcon struct {
	ID     string  `json:"id"` // "prefix:name"
	Set    string  `json:"set"`
	Name   string  `json:"name"`
	Body   string  `json:"body"`
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	// Recolorable icons draw in currentColor, which the editor sets; the
	// others (emoji) keep their own colours.
	Recolorable bool `json:"recolorable"`
}

type ClipartSet struct {
	Name    string `json:"name"`
	Author  string `json:"author,omitempty"`
	License string `json:"license"`
}

type clipartSearchResult struct {
	Icons []ClipartIcon         `json:"icons"`
	Sets  map[string]ClipartSet `json:"sets"`
}

// Search answers GET /api/clipart/search?q=&limit=.
func (h *ClipartHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}
	if len(q) > 100 {
		q = q[:100]
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	// Iconify takes 32 to 999.
	if limit < 32 {
		limit = 64
	}
	if limit > 128 {
		limit = 128
	}
	key := strings.ToLower(q) + "|" + strconv.Itoa(limit)
	if res, ok := h.cached(key); ok {
		c.JSON(http.StatusOK, res)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	res, err := h.search(ctx, q, limit)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Clipart search failed: " + err.Error()})
		return
	}
	h.store(key, res)
	c.JSON(http.StatusOK, res)
}

func (h *ClipartHandler) cached(key string) (clipartSearchResult, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.cache[key]
	if !ok || time.Since(e.at) > clipartCacheTTL {
		return clipartSearchResult{}, false
	}
	return e.res, true
}

func (h *ClipartHandler) store(key string, res clipartSearchResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.cache) >= clipartCacheEntries {
		h.cache = map[string]clipartCacheEntry{}
	}
	h.cache[key] = clipartCacheEntry{at: time.Now(), res: res}
}

type iconifyInfo struct {
	Name   string `json:"name"`
	Author struct {
		Name string `json:"name"`
	} `json:"author"`
	License struct {
		Title string `json:"title"`
		SPDX  string `json:"spdx"`
	} `json:"license"`
}

type iconifySearchResponse struct {
	Icons       []string               `json:"icons"`
	Collections map[string]iconifyInfo `json:"collections"`
}

func (h *ClipartHandler) search(ctx context.Context, q string, limit int) (clipartSearchResult, error) {
	params := url.Values{}
	params.Set("query", q)
	params.Set("limit", strconv.Itoa(limit))
	params.Set("prefixes", strings.Join(clipartSets, ","))
	var sr iconifySearchResponse
	if err := h.getJSON(ctx, "/search?"+params.Encode(), &sr); err != nil {
		return clipartSearchResult{}, err
	}

	allowed := map[string]bool{}
	for _, p := range clipartSets {
		allowed[p] = true
	}
	res := clipartSearchResult{Icons: []ClipartIcon{}, Sets: map[string]ClipartSet{}}
	var order [][2]string
	byPrefix := map[string][]string{}
	for _, id := range sr.Icons {
		prefix, name, ok := strings.Cut(id, ":")
		if !ok || !allowed[prefix] || !clipartNameRe.MatchString(name) {
			continue
		}
		info, ok := sr.Collections[prefix]
		if !ok || !clipartLicenses[info.License.SPDX] {
			continue
		}
		if _, seen := res.Sets[prefix]; !seen {
			res.Sets[prefix] = ClipartSet{Name: info.Name, Author: info.Author.Name, License: info.License.SPDX}
		}
		order = append(order, [2]string{prefix, name})
		byPrefix[prefix] = append(byPrefix[prefix], name)
	}

	// One request per icon set for the icons' data, all at once.
	type setData struct {
		prefix string
		data   *iconifyJSON
		err    error
	}
	results := make(chan setData, len(byPrefix))
	for prefix, names := range byPrefix {
		go func(prefix string, names []string) {
			var data iconifyJSON
			err := h.getJSON(ctx, "/"+prefix+".json?icons="+url.QueryEscape(strings.Join(names, ",")), &data)
			results <- setData{prefix, &data, err}
		}(prefix, names)
	}
	sets := map[string]*iconifyJSON{}
	var firstErr error
	for range byPrefix {
		r := <-results
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		sets[r.prefix] = r.data
	}
	if len(sets) == 0 && firstErr != nil {
		return clipartSearchResult{}, firstErr
	}

	for _, pn := range order {
		data := sets[pn[0]]
		if data == nil {
			continue
		}
		icon, ok := data.resolve(pn[1])
		if !ok {
			continue
		}
		icon.ID = pn[0] + ":" + pn[1]
		icon.Set = pn[0]
		icon.Name = pn[1]
		res.Icons = append(res.Icons, icon)
	}
	return res, nil
}

func (h *ClipartHandler) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("iconify answered %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(v)
}

// iconifyJSON is Iconify's icon set format (github.com/iconify/iconify,
// @iconify/types). Its root carries the set's default dimensions.
type iconifyJSON struct {
	Prefix  string                  `json:"prefix"`
	Icons   map[string]iconifyEntry `json:"icons"`
	Aliases map[string]iconifyEntry `json:"aliases"`
	iconifyProps
}

type iconifyProps struct {
	Left   *float64 `json:"left,omitempty"`
	Top    *float64 `json:"top,omitempty"`
	Width  *float64 `json:"width,omitempty"`
	Height *float64 `json:"height,omitempty"`
	Rotate int      `json:"rotate,omitempty"`
	HFlip  bool     `json:"hFlip,omitempty"`
	VFlip  bool     `json:"vFlip,omitempty"`
}

type iconifyEntry struct {
	Body   string `json:"body,omitempty"`
	Parent string `json:"parent,omitempty"`
	iconifyProps
}

// resolve follows an alias to its icon, merging dimensions (the alias's own
// win) and transformations (flips toggle, turns add up) on the way, as
// @iconify/utils' getIconData does, then applies the transformations to the
// body as its iconToSVG does.
func (d *iconifyJSON) resolve(name string) (ClipartIcon, bool) {
	var acc iconifyProps
	merge := func(p iconifyProps) {
		if acc.Left == nil {
			acc.Left = p.Left
		}
		if acc.Top == nil {
			acc.Top = p.Top
		}
		if acc.Width == nil {
			acc.Width = p.Width
		}
		if acc.Height == nil {
			acc.Height = p.Height
		}
		acc.Rotate += p.Rotate
		acc.HFlip = acc.HFlip != p.HFlip
		acc.VFlip = acc.VFlip != p.VFlip
	}
	body := ""
	found := false
	for depth := 0; depth < 10; depth++ {
		if icon, ok := d.Icons[name]; ok {
			merge(icon.iconifyProps)
			body = icon.Body
			found = true
			break
		}
		alias, ok := d.Aliases[name]
		if !ok || alias.Parent == "" {
			return ClipartIcon{}, false
		}
		merge(alias.iconifyProps)
		name = alias.Parent
	}
	if !found || body == "" {
		return ClipartIcon{}, false
	}
	// The set's defaults, then Iconify's.
	root := d.iconifyProps
	root.Rotate, root.HFlip, root.VFlip = 0, false, false
	merge(root)
	num := func(p *float64, def float64) float64 {
		if p == nil {
			return def
		}
		return *p
	}
	left, top := num(acc.Left, 0), num(acc.Top, 0)
	width, height := num(acc.Width, 16), num(acc.Height, 16)
	f := func(v float64) string { return strconv.FormatFloat(v+0, 'f', -1, 64) } // +0: no "-0"

	var transforms []string
	rotation := acc.Rotate
	if acc.HFlip {
		if acc.VFlip {
			rotation += 2
		} else {
			transforms = append(transforms, "translate("+f(width+left)+" "+f(-top)+")", "scale(-1 1)")
			left, top = 0, 0
		}
	} else if acc.VFlip {
		transforms = append(transforms, "translate("+f(-left)+" "+f(height+top)+")", "scale(1 -1)")
		left, top = 0, 0
	}
	rotation = ((rotation % 4) + 4) % 4
	switch rotation {
	case 1:
		v := height/2 + top
		transforms = append([]string{"rotate(90 " + f(v) + " " + f(v) + ")"}, transforms...)
	case 2:
		transforms = append([]string{"rotate(180 " + f(width/2+left) + " " + f(height/2+top) + ")"}, transforms...)
	case 3:
		v := width/2 + left
		transforms = append([]string{"rotate(-90 " + f(v) + " " + f(v) + ")"}, transforms...)
	}
	if rotation%2 == 1 {
		left, top = top, left
		width, height = height, width
	}
	if len(transforms) > 0 {
		body = wrapSVGContent(body, `<g transform="`+strings.Join(transforms, " ")+`">`, "</g>")
	}
	return ClipartIcon{
		Body: body, Left: left, Top: top, Width: width, Height: height,
		Recolorable: strings.Contains(body, "currentColor"),
	}, true
}

// wrapSVGContent wraps an icon body in a group, keeping its <defs> outside
// it (@iconify/utils' wrapSVGContent).
func wrapSVGContent(body, start, end string) string {
	defs := ""
	for {
		i := strings.Index(body, "<defs")
		if i < 0 {
			break
		}
		open := strings.Index(body[i:], ">")
		closeTag := strings.Index(body, "</defs")
		if open < 0 || closeTag < 0 || closeTag < i+open {
			break
		}
		closeEnd := strings.Index(body[closeTag:], ">")
		if closeEnd < 0 {
			break
		}
		defs += strings.TrimSpace(body[i+open+1 : closeTag])
		body = strings.TrimSpace(body[:i]) + body[closeTag+closeEnd+1:]
	}
	content := start + body + end
	if defs != "" {
		return "<defs>" + defs + "</defs>" + content
	}
	return content
}
