package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

// callREST runs a REST handler on behalf of an MCP call, so a tool that
// exposes an existing endpoint behaves exactly like it: same validation, same
// pipeline, same answers. The synthetic request carries the caller's context
// keys (userId, teamId and so on, with the admin flags already cleared by
// MCPAuthRequired). body is sent as JSON when not nil.
//
// It returns the HTTP status and the decoded JSON answer.
func callREST(c *gin.Context, handler gin.HandlerFunc, method, target string, body any) (int, map[string]any, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(raw)
	}
	w := httptest.NewRecorder()
	rc, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c != nil && c.Request != nil {
		req = req.WithContext(c.Request.Context())
		req.Host = c.Request.Host
	}
	rc.Request = req
	if c != nil {
		for k, v := range c.Keys {
			rc.Set(k, v)
		}
	}
	handler(rc)

	out := map[string]any{}
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			return w.Code, nil, fmt.Errorf("unexpected answer (HTTP %d)", w.Code)
		}
	}
	return w.Code, out, nil
}

// restResult turns a callREST answer into a tool result: the JSON body, with
// any non-2xx status reported as a tool error.
func restResult(status int, out map[string]any, err error) (any, bool) {
	if err != nil {
		return map[string]string{"error": err.Error()}, true
	}
	if status < 200 || status >= 300 {
		if _, ok := out["error"]; !ok {
			out["error"] = http.StatusText(status)
		}
		return out, true
	}
	return out, false
}
