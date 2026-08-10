package viewerchat

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	maxRequestBytes = 64 * 1024
	maxMessages     = 40
	maxContentRunes = 8000
)

// Request is the JSON body Persona posts (plus optional day context).
type Request struct {
	Messages []clientMsg    `json:"messages"`
	Context  map[string]any `json:"context,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Day      int            `json:"day,omitempty"`
}

type clientMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// Handler serves POST chat for one trip.
type Handler struct {
	Agent *Agent
}

// ServeHTTP runs one chat turn as an SSE stream.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, tripID string) {
	if h == nil || h.Agent == nil {
		http.Error(w, "chat unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limited := io.LimitReader(r.Body, maxRequestBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(body) > maxRequestBytes {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}

	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	msgs, err := normalizeMessages(req.Messages)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	day := req.Day
	if day == 0 {
		day = dayFromContext(req.Context)
	}
	if day == 0 {
		day = dayFromContext(req.Metadata)
	}

	sw, err := newSSEWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Keep the stream alive with SSE comments only — status data events were
	// ending up in Persona's transcript (parseSSEEvent null = unhandled).
	_ = sw.ping()

	stopPing := make(chan struct{})
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-r.Context().Done():
				return
			case <-t.C:
				if err := sw.ping(); err != nil {
					return
				}
			}
		}
	}()
	defer close(stopPing)

	res, err := h.Agent.run(r.Context(), TurnInput{
		TripID:   tripID,
		Messages: msgs,
		Day:      day,
	}, sw.send)
	if err != nil {
		log.Printf("viewerchat trip=%s: %v", tripID, err)
		_ = sw.send(Event{Type: "error", Error: err.Error()})
		_ = sw.send(Event{Type: "done", Done: true})
		return
	}
	if res.TripUpdated {
		_ = sw.send(Event{Type: "trip_updated"})
	}
	_ = sw.send(Event{Type: "done", Done: true})
}

func normalizeMessages(in []clientMsg) ([]ClientMessage, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("messages required")
	}
	if len(in) > maxMessages {
		in = in[len(in)-maxMessages:]
	}
	out := make([]ClientMessage, 0, len(in))
	for _, m := range in {
		text, err := contentToString(m.Content)
		if err != nil {
			return nil, err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if len([]rune(text)) > maxContentRunes {
			return nil, fmt.Errorf("message too long")
		}
		out = append(out, ClientMessage{
			Role:    strings.TrimSpace(m.Role),
			Content: text,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("messages required")
	}
	return out, nil
}

func contentToString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if t, ok := p["text"].(string); ok {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(t)
			}
		}
		return b.String(), nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if t, ok := obj["text"].(string); ok {
			return t, nil
		}
	}
	return "", fmt.Errorf("unsupported message content")
}

func dayFromContext(ctx map[string]any) int {
	if ctx == nil {
		return 0
	}
	for _, key := range []string{"day", "currentDay", "current_day"} {
		switch v := ctx[key].(type) {
		case float64:
			return int(v)
		case json.Number:
			n, _ := v.Int64()
			return int(n)
		case int:
			return v
		}
	}
	return 0
}
