package httpserver

import (
	"net/http"
	"strings"
)

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
	s.chat.ServeHTTP(w, r, id)
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
