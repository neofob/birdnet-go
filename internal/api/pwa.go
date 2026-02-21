package api

import (
	"github.com/labstack/echo/v4"
)

// registerPWARoutes registers routes for PWA support files.
// The manifest and service worker must be served from root paths
// so the service worker scope covers the entire application.
func (s *Server) registerPWARoutes() {
	// Serve manifest.webmanifest from root path
	s.echo.GET("/manifest.webmanifest", func(c echo.Context) error {
		return s.staticServer.handlePWAFile(c, "manifest.webmanifest")
	})

	// Serve service worker from root path with Service-Worker-Allowed header
	s.echo.GET("/sw.js", func(c echo.Context) error {
		c.Response().Header().Set("Service-Worker-Allowed", "/")
		return s.staticServer.handlePWAFile(c, "sw.js")
	})
}

// handlePWAFile serves a PWA file from the static file server (dev or embedded).
// These files are stored in frontend/static/ and built into dist/.
// PWA files have fixed (non-hashed) names, so we override the default
// immutable cache headers that serveFileContent sets for Vite-hashed assets.
func (sfs *StaticFileServer) handlePWAFile(c echo.Context, filename string) error {
	sfs.initDevMode()
	if sfs.devMode {
		return sfs.serveFromDisk(c, filename)
	}
	// Set cache headers before serveFromEmbed — prevents the 1-year immutable
	// cache that serveFileContent applies to content-hashed Vite bundles.
	c.Response().Header().Set("Cache-Control", "no-cache")
	return sfs.serveFromEmbed(c, filename)
}
