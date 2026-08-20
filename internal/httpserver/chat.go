package httpserver

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

// chatHTTP is the optional in-viewer chat agent (wired in Phase 3).
type chatHTTP interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request, tripID, userSub string)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.sessionFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in required"})
		return
	}
	if !s.chatAllowed(sess) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "chat not authorized for this account"})
		return
	}
	if s.chat == nil || s.cfg.OpenAIAPIKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "chat unavailable (OpenAI not configured)"})
		return
	}
	s.chat.ServeHTTP(w, r, id, strings.TrimSpace(sess.Sub))
}

func (s *Server) handleChatFeedback(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.sessionFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in required"})
		return
	}
	if !s.chatAllowed(sess) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "chat not authorized for this account"})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limited := io.LimitReader(r.Body, 16*1024+1)
	body, err := io.ReadAll(limited)
	if err != nil || len(body) > 16*1024 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var req struct {
		Vote          string `json:"vote"` // up | down
		UserText      string `json:"user_text"`
		AssistantText string `json:"assistant_text"`
		Day           int    `json:"day"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	vote := strings.ToLower(strings.TrimSpace(req.Vote))
	if vote != "up" && vote != "down" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "vote must be up or down"})
		return
	}
	log.Printf("viewerchat feedback trip=%s day=%d sub=%s vote=%s user=%q assistant=%q",
		id, req.Day, strings.TrimSpace(sess.Sub), vote,
		truncateForLog(req.UserText, 120), truncateForLog(req.AssistantText, 160))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"vote": vote,
	})
}

func truncateForLog(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

func (s *Server) chatAllowed(sess sessionCookie) bool {
	email := strings.ToLower(strings.TrimSpace(sess.Email))
	sub := strings.TrimSpace(sess.Sub)
	for _, e := range s.cfg.ChatAllowedEmails {
		if e != "" && e == email {
			return true
		}
	}
	for _, id := range s.cfg.ChatAllowedSubs {
		if id != "" && id == sub {
			return true
		}
	}
	return false
}

func (s *Server) chatEnabledFor(sess sessionCookie) bool {
	return s.cfg.OpenAIAPIKey != "" && s.chat != nil && s.chatAllowed(sess)
}
