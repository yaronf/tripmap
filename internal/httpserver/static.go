package httpserver

import (
	"embed"
	"net/http"
)

//go:embed static/favicon.png
var staticFS embed.FS

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	b, err := staticFS.ReadFile("static/favicon.png")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}
