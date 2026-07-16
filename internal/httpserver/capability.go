package httpserver

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yaronf/tripmap/internal/store"
)

const maxNotesBytes = 64 * 1024

func (s *Server) handleCapability(w http.ResponseWriter, r *http.Request) {
	// /t/{id}/{token} or /t/{id}/{token}/… 
	rest := strings.TrimPrefix(r.URL.Path, "/t/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	id, token := parts[0], parts[1]
	rel := ""
	if len(parts) == 3 {
		rel = parts[2]
	}

	// Redirect /t/{id}/{token} → /t/{id}/{token}/
	if rel == "" && !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
		return
	}

	if err := s.verifyCapability(r, id, token); err != nil {
		http.NotFound(w, r)
		return
	}

	if rel == "api/notes" || strings.HasPrefix(rel, "api/notes/") {
		s.handleNotes(w, r, id)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, ct, err := s.store.GetBundleObject(r.Context(), id, rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func (s *Server) verifyCapability(r *http.Request, id, token string) error {
	meta, err := s.store.GetMeta(r.Context(), id)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(token))
	got := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(got), []byte(meta.TokenHash)) != 1 {
		return fmt.Errorf("bad token")
	}
	return nil
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