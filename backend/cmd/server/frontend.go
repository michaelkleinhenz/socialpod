package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// all: keeps files starting with _ or . (Vite emits chunks such as
// __vite-browser-external-*.js), which a plain pattern leaves out.
//
//go:embed all:dist
var frontendFS embed.FS

func serveFrontend(r *gin.Engine) {
	distFS, _ := fs.Sub(frontendFS, "dist")
	fileServer := http.FileServer(http.FS(distFS))

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// API routes that don't match should return JSON 404, not the SPA.
		if strings.HasPrefix(path, "/api/") || path == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			return
		}

		// Serve static assets directly
		if strings.Contains(path, ".") {
			c.FileFromFS(path, http.FS(distFS))
			return
		}

		// SPA fallback: serve index.html for all non-file routes
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
