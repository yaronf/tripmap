package httpserver

import (
	"embed"
	"net/http"
)

//go:embed static/favicon.svg
var staticFS embed.FS

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	b, err := staticFS.ReadFile("static/favicon.svg")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}
