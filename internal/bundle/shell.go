package bundle

import (
	"encoding/json"
	"fmt"
	"html"
	"path"
	"strings"
)

// ViewerShell returns an embedded viewer file (app.js, CSS, icons, …).
// Trip data (trip.json, geo/, images/, sw.js) is not served from embed.
func ViewerShell(rel string) ([]byte, string, bool) {
	rel = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(rel)), "/")
	if rel == "." || rel == "" {
		rel = "index.html"
	}
	switch {
	case rel == "trip.json", rel == "sw.js", rel == "manifest.webmanifest":
		return nil, "", false
	case strings.HasPrefix(rel, "geo/"), strings.HasPrefix(rel, "images/"):
		return nil, "", false
	}
	b, err := viewerFS.ReadFile("viewer/" + rel)
	if err != nil {
		return nil, "", false
	}
	return b, shellContentType(rel), true
}

func shellContentType(rel string) string {
	switch {
	case strings.HasSuffix(rel, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(rel, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(rel, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(rel, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(rel, ".png"):
		return "image/png"
	case strings.HasSuffix(rel, ".webmanifest"):
		return "application/manifest+json"
	default:
		return "application/octet-stream"
	}
}

// StampIndexHTML injects per-trip <title> and Open Graph tags.
func StampIndexHTML(raw []byte, pageTitle, desc string) ([]byte, error) {
	pageTitle = strings.TrimSpace(pageTitle)
	if pageTitle == "" {
		pageTitle = "Trip"
	} else if !strings.Contains(strings.ToLower(pageTitle), "itinerary") {
		pageTitle = pageTitle + " Itinerary"
	}
	desc = strings.TrimSpace(desc)
	if desc == "" {
		desc = pageTitle
	}
	meta := fmt.Sprintf(`<title>%s</title>
  <meta name="description" content="%s" />
  <meta property="og:title" content="%s" />
  <meta property="og:description" content="%s" />
  <meta property="og:type" content="website" />`,
		html.EscapeString(pageTitle),
		html.EscapeString(desc),
		html.EscapeString(pageTitle),
		html.EscapeString(desc),
	)
	out := strings.Replace(string(raw), "<title>Trip</title>", meta, 1)
	if out == string(raw) {
		return nil, fmt.Errorf("viewer index.html missing <title>Trip</title> placeholder")
	}
	return []byte(out), nil
}

// StampIndexFromTripJSON stamps index.html using trip.json title fields.
func StampIndexFromTripJSON(raw, tripJSON []byte) ([]byte, error) {
	var tj struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(tripJSON, &tj); err != nil {
		return nil, err
	}
	return StampIndexHTML(raw, tj.Title, tj.Description)
}
