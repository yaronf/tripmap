package viewerchat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event is one SSE data payload for Persona (parseSSEEvent maps these).
type Event struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
	Done  bool   `json:"done,omitempty"`
}

type sseWriter struct {
	mu sync.Mutex
	w  http.ResponseWriter
	f  http.Flusher
}

func newSSEWriter(w http.ResponseWriter) (*sseWriter, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// Comment + flush so proxies/browsers see an EventStream immediately
	// (headers alone are often buffered until the first body byte).
	if _, err := fmt.Fprintf(w, ": ok\n\n"); err != nil {
		return nil, err
	}
	f.Flush()
	return &sseWriter{w: w, f: f}, nil
}

func (s *sseWriter) send(ev Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", b); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

func (s *sseWriter) ping() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := fmt.Fprintf(s.w, ": ping %d\n\n", time.Now().Unix()); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}
