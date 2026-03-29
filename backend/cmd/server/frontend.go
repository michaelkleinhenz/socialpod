package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist/*
var frontendFS embed.FS

func serveFrontend(r *gin.Engine) {
	distFS, _ := fs.Sub(frontendFS, "dist")
	fileServer := http.FileServer(http.FS(distFS))

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

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
