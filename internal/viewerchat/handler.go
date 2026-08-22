package viewerchat

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	Log   *slog.Logger
}

// ServeHTTP runs one chat turn as an SSE stream.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, tripID, userSub string) {
	if h == nil || h.Agent == nil {
		http.Error(w, "chat unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log := h.Log
	if log == nil {
		log = slog.Default()
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

	requestID := newRequestID()
	tl := turnLogger{log: log, requestID: requestID, tripID: tripID, sub: userSub, day: day}
	lastUser := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.EqualFold(msgs[i].Role, "user") {
			lastUser = msgs[i].Content
			break
		}
	}
	turnStart := time.Now()
	tl.with(
		"msg_count", len(msgs),
		"body_bytes", len(body),
		"user", truncateRunes(lastUser, 120),
		"model", h.Agent.model,
		"user_agent", truncateRunes(r.UserAgent(), 160),
	).Info("turn_start")

	_ = sw.send(Event{Type: "status", Status: "starting"})

	res, err := h.Agent.Run(r.Context(), TurnInput{
		TripID:   tripID,
		UserSub:  userSub,
		Messages: msgs,
		Day:      day,
	}, sw.send, tl)
	totalMS := time.Since(turnStart).Milliseconds()
	if err != nil {
		tl.with("total_ms", totalMS, "trip_updated", res.TripUpdated).Error("turn_end", "outcome", "error", "error", truncateRunes(err.Error(), 300))
		_ = sw.send(Event{Type: "error", Error: err.Error()})
		_ = sw.send(Event{Type: "done", Done: true})
		return
	}
	tl.with("total_ms", totalMS, "trip_updated", res.TripUpdated).Info("turn_end", "outcome", "done")
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
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		text, err := contentToString(m.Content)
		if err != nil {
			return nil, err
		}
		text = truncateRunes(text, maxContentRunes)
		if text == "" {
			continue
		}
		out = append(out, ClientMessage{Role: role, Content: text})
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
				b.WriteString(t)
			}
		}
		return b.String(), nil
	}
	return strings.TrimSpace(string(raw)), nil
}

func dayFromContext(m map[string]any) int {
	if m == nil {
		return 0
	}
	for _, key := range []string{"day", "current_day", "dayNumber"} {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case json.Number:
			i, _ := n.Int64()
			return int(i)
		case string:
			var i int
			_, _ = fmt.Sscanf(n, "%d", &i)
			return i
		}
	}
	return 0
}
