package httpserver

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/client"
)

func (s *Server) authEnabled() bool {
	return strings.TrimSpace(s.cfg.DescopeProjectID) != "" &&
		strings.TrimSpace(s.cfg.SessionSecret) != ""
}

func (s *Server) descope() (*client.DescopeClient, error) {
	s.descopeInit.Do(func() {
		s.descopeCli, s.descopeErr = client.NewWithConfig(&client.Config{
			ProjectID: s.cfg.DescopeProjectID,
		})
	})
	return s.descopeCli, s.descopeErr
}

func (s *Server) authCallbackURI() string {
	base := strings.TrimRight(s.cfg.PublicBaseURL, "/")
	if base == "" {
		return ""
	}
	return base + "/auth/callback"
}

// authCallbackURIForRequest prefers the browser Host on loopback so cookies and
// Descope redirectURL stay on the same site (localhost vs 127.0.0.1).
func (s *Server) authCallbackURIForRequest(r *http.Request) string {
	host := r.Host
	if host != "" && isLoopbackHost(host) {
		return requestScheme(r) + "://" + host + "/auth/callback"
	}
	return s.authCallbackURI()
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if !s.authEnabled() {
		http.Error(w, "login not configured", http.StatusServiceUnavailable)
		return
	}
	redirect := s.authCallbackURIForRequest(r)
	if redirect == "" {
		http.Error(w, "PUBLIC_BASE_URL required", http.StatusServiceUnavailable)
		return
	}

	pending := loginPendingCookie{
		ReturnTo: sanitizeReturnTo(r.URL.Query().Get("return_to")),
		Exp:      time.Now().Add(loginCookieTTL).Unix(),
	}
	if err := s.setSignedCookie(w, r, loginCookieName, pending, loginCookieTTL); err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", err)
		return
	}

	dc, err := s.descope()
	if err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", fmt.Errorf("descope client: %w", err))
		return
	}
	// Pass nil ResponseWriter so we control the redirect (and Set-Cookie sticks).
	authURL, err := dc.Auth.OAuth().SignUpOrIn(r.Context(), descope.OAuthGoogle, redirect, "", r, nil, nil)
	if err != nil || authURL == "" {
		writeAuthErr(w, http.StatusBadGateway, "login failed", fmt.Errorf("oauth start: %w", err))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"/>
<link rel="icon" href="/favicon.png" type="image/png"/>
<meta http-equiv="refresh" content="0;url=%s"/>
<title>Continuing to Google…</title></head><body>
<p>Continuing to Google…</p>
<script>location.replace(%q)</script>
</body></html>`, htmlEscape(authURL), authURL)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if !s.authEnabled() {
		http.Error(w, "login not configured", http.StatusServiceUnavailable)
		return
	}
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		log.Printf("descope auth denied: error=%q desc=%q", errMsg, r.URL.Query().Get("error_description"))
		http.Error(w, "login failed", http.StatusBadRequest)
		return
	}

	var pending loginPendingCookie
	if err := s.readSignedCookie(r, loginCookieName, &pending); err != nil {
		log.Printf("descope login cookie: %v", err)
		http.Error(w, "missing or expired login state — try Continue with Google again", http.StatusBadRequest)
		return
	}
	s.clearCookie(w, r, loginCookieName)
	if time.Now().Unix() > pending.Exp {
		http.Error(w, "login state expired", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	dc, err := s.descope()
	if err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", fmt.Errorf("descope client: %w", err))
		return
	}
	info, err := dc.Auth.OAuth().ExchangeToken(r.Context(), code, nil)
	if err != nil {
		writeAuthErr(w, http.StatusBadGateway, "login failed", fmt.Errorf("token exchange: %w", err))
		return
	}

	email, name, sub := "", "", ""
	if info != nil && info.User != nil {
		email = strings.TrimSpace(info.User.Email)
		name = strings.TrimSpace(info.User.Name)
		sub = strings.TrimSpace(info.User.UserID)
	}
	if sub == "" && info != nil && info.SessionToken != nil {
		sub = strings.TrimSpace(info.SessionToken.ID)
	}
	if sub == "" {
		writeAuthErr(w, http.StatusBadGateway, "login failed", fmt.Errorf("missing user id"))
		return
	}

	if !s.identityAllowed(email) {
		log.Printf("descope auth denied: not on allowlist email=%q sub=%q", email, sub)
		http.Error(w, "This Google account is not authorized for tripmap.", http.StatusForbidden)
		return
	}

	sess := sessionCookie{
		Sub:   sub,
		Email: email,
		Name:  name,
		Exp:   time.Now().Add(sessionCookieTTL).Unix(),
	}
	if err := s.setSignedCookie(w, r, sessionCookieName, sess, sessionCookieTTL); err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", err)
		return
	}
	http.Redirect(w, r, sanitizeReturnTo(pending.ReturnTo), http.StatusFound)
}

func (s *Server) identityAllowed(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, e := range s.cfg.AllowedEmails {
		if e != "" && e == email {
			return true
		}
	}
	return false
}

func (s *Server) sessionAllowed(sess sessionCookie) bool {
	return s.identityAllowed(sess.Email)
}

func writeAuthErr(w http.ResponseWriter, status int, public string, err error) {
	if err != nil {
		log.Printf("auth: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": public})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionFromRequest(r)
	if !ok || !s.sessionAllowed(sess) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"sub":           sess.Sub,
		"email":         sess.Email,
		"name":          sess.Name,
		"exp":           sess.Exp,
		"chat_enabled":  s.chatEnabledFor(sess),
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.clearCookie(w, r, sessionCookieName)
	s.clearCookie(w, r, loginCookieName)
	http.Redirect(w, r, "/", http.StatusFound)
}
