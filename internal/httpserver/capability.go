package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yaronf/tripmap/internal/bundle"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/store"
)

const maxNotesBytes = 64 * 1024

// handleSessionTrip serves /me/trips/{id}/… to signed-in Hellō users.
func (s *Server) handleSessionTrip(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/me/trips/")
	if rest == "" || rest == "/" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	if id == "" || itinerary.ValidateID(id) != nil {
		http.NotFound(w, r)
		return
	}
	rel := ""
	if len(parts) == 2 {
		rel = parts[1]
	}

	sess, authed := s.sessionFromRequest(r)
	if !authed {
		if strings.HasPrefix(rel, "api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in required"})
			return
		}
		// Chrome fetches link[rel=manifest] (and often the icon) without cookies.
		// Redirecting those to the HTML login page yields "Manifest: Syntax error".
		if publicBundleRel(rel) {
			ok, err := s.store.Exists(r.Context(), id)
			if err != nil || !ok {
				http.NotFound(w, r)
				return
			}
			s.serveTripBundle(w, r, id, rel)
			return
		}
		http.Redirect(w, r, "/auth/hello/login?return_to="+url.QueryEscape(r.URL.Path), http.StatusFound)
		return
	}
	_ = sess

	if rel == "" && !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
		return
	}

	ok, err := s.store.Exists(r.Context(), id)
	if err != nil || !ok {
		http.NotFound(w, r)
		return
	}
	s.serveTripBundle(w, r, id, rel)
}

// publicBundleRel is true for install-surface assets that must be readable
// without a session cookie. Keep this list minimal (no trip.json / geo / photos).
func publicBundleRel(rel string) bool {
	switch strings.TrimSpace(rel) {
	case "manifest.webmanifest", "icon.png", "icon.svg":
		return true
	default:
		return false
	}
}

func (s *Server) serveTripBundle(w http.ResponseWriter, r *http.Request, id, rel string) {
	if rel == "api/notes" || strings.HasPrefix(rel, "api/notes/") {
		s.handleNotes(w, r, id)
		return
	}
	if rel == "api/chat/feedback" {
		s.handleChatFeedback(w, r, id)
		return
	}
	if rel == "api/chat" || strings.HasPrefix(rel, "api/chat/") {
		s.handleChat(w, r, id)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, ct, ok, err := s.viewerFromImage(r.Context(), id, rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !ok {
		body, ct, err = s.store.GetBundleObject(r.Context(), id, rel)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// viewerFromImage serves the PWA shell from the running binary so S3 copies of
// app.js cannot drift from a later itinerary regen.
func (s *Server) viewerFromImage(ctx context.Context, id, rel string) ([]byte, string, bool, error) {
	raw, ct, ok := bundle.ViewerShell(rel)
	if !ok {
		return nil, "", false, nil
	}
	if rel != "" && rel != "index.html" && rel != "/" {
		return raw, ct, true, nil
	}
	tj, _, err := s.store.GetBundleObject(ctx, id, "trip.json")
	if err != nil {
		return nil, "", false, err
	}
	stamped, err := bundle.StampIndexFromTripJSON(raw, tj)
	if err != nil {
		return nil, "", false, err
	}
	return stamped, ct, true, nil
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		b, err := s.store.GetNotes(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "private, max-age=0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
		if len(b) == 0 || b[len(b)-1] != '\n' {
			_, _ = w.Write([]byte("\n"))
		}
	case http.MethodPut:
		defer r.Body.Close()
		limited := io.LimitReader(r.Body, maxNotesBytes+1)
		body, err := io.ReadAll(limited)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if len(body) > maxNotesBytes {
			writeErr(w, http.StatusRequestEntityTooLarge, fmt.Errorf("notes exceed %d bytes", maxNotesBytes))
			return
		}
		var doc store.NotesDoc
		if err := json.Unmarshal(body, &doc); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid notes json: %w", err))
			return
		}
		if doc.Days == nil {
			doc.Days = map[string]string{}
		}
		doc.UpdatedAt = time.Now().UTC()
		out, err := json.Marshal(doc)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, '\n')
		if err := s.store.PutNotes(r.Context(), id, out); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
