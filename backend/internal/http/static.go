package http

import (
	"io/fs"
	nethttp "net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"gist/backend/internal/webui"
	"gist/backend/pkg/logger"
)

func registerStatic(e *echo.Echo, dir string) {
	// A binary built with embedded assets always serves those; the on-disk
	// directory is only used by development builds.
	if staticFS := webui.FS(); staticFS != nil {
		serveStaticFS(e, staticFS, "embedded")
		return
	}

	if dir == "" {
		return
	}
	indexPath := filepath.Join(dir, "index.html")
	info, err := os.Stat(indexPath)
	if err != nil || info.IsDir() {
		logger.Warn("static index missing", "module", "http", "action", "request", "resource", "http", "result", "failed", "path", indexPath)
		return
	}

	serveStaticFS(e, os.DirFS(dir), dir)
}

func serveStaticFS(e *echo.Echo, staticFS fs.FS, source string) {
	indexHTML, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		logger.Warn("static index missing", "module", "http", "action", "request", "resource", "http", "result", "failed", "source", source)
		return
	}

	logger.Info("static assets enabled", "module", "http", "action", "request", "resource", "http", "result", "ok", "source", source)

	fileServer := nethttp.FileServer(nethttp.FS(staticFS))

	e.GET("/*", func(c echo.Context) error {
		requestPath := c.Request().URL.Path
		if requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") {
			return echo.ErrNotFound
		}

		if requestPath != "/" {
			cleanPath := strings.TrimPrefix(path.Clean(requestPath), "/")
			if cleanPath != "." && cleanPath != "" {
				if fileInfo, statErr := fs.Stat(staticFS, cleanPath); statErr == nil && !fileInfo.IsDir() {
					setStaticHeaders(c.Response().Header(), cleanPath)
					logger.Debug("static file served", "module", "http", "action", "fetch", "resource", "http", "result", "ok", "path", requestPath)
					fileServer.ServeHTTP(c.Response(), c.Request())
					return nil
				}

				if shouldBypassSPA(cleanPath) {
					logger.Debug("static asset missing", "module", "http", "action", "fetch", "resource", "http", "result", "failed", "path", requestPath)
					return echo.ErrNotFound
				}
			}
		}

		setStaticHeaders(c.Response().Header(), "index.html")
		logger.Debug("static index served", "module", "http", "action", "fetch", "resource", "http", "result", "ok", "path", requestPath)
		return c.Blob(nethttp.StatusOK, "text/html; charset=utf-8", indexHTML)
	})
}

func shouldBypassSPA(cleanPath string) bool {
	if strings.HasPrefix(cleanPath, "assets/") {
		return true
	}
	return path.Ext(cleanPath) != ""
}

func setStaticHeaders(header nethttp.Header, cleanPath string) {
	switch cleanPath {
	case "index.html", "sw.js", "manifest.webmanifest":
		header.Set("Cache-Control", "no-cache")
	}
}
