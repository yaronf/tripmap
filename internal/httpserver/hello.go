package httpserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	helloIssuer       = "https://issuer.hello.coop"
	oauthCookieName   = "tripmap_oauth"
	sessionCookieName = "tripmap_session"
	oauthCookieTTL    = 10 * time.Minute
	sessionCookieTTL  = 7 * 24 * time.Hour
)

type oauthCookie struct {
	State        string `json:"state"`
	Nonce        string `json:"nonce"`
	CodeVerifier string `json:"cv"`
	RedirectURI  string `json:"ru,omitempty"`
	ReturnTo     string `json:"rt,omitempty"`
	Exp          int64  `json:"exp"`
}

type sessionCookie struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	Exp   int64  `json:"exp"`
}

func (s *Server) helloEnabled() bool {
	return strings.TrimSpace(s.cfg.HelloClientID) != "" &&
		strings.TrimSpace(s.cfg.HelloSessionSecret) != ""
}

func (s *Server) helloRedirectURI() string {
	if u := strings.TrimSpace(s.cfg.HelloRedirectURI); u != "" {
		return u
	}
	base := strings.TrimRight(s.cfg.PublicBaseURL, "/")
	if base == "" {
		return ""
	}
	return base + "/auth/hello/callback"
}

// helloRedirectURIForRequest prefers the browser Host on loopback so cookies and
// Hellō redirect_uri stay on the same site (localhost vs 127.0.0.1).
func (s *Server) helloRedirectURIForRequest(r *http.Request) string {
	host := r.Host
	if host != "" && isLoopbackHost(host) {
		return requestScheme(r) + "://" + host + "/auth/hello/callback"
	}
	return s.helloRedirectURI()
}

func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return "https"
	}
	return "http"
}

func (s *Server) sessionKey() []byte {
	sum := sha256.Sum256([]byte(s.cfg.HelloSessionSecret))
	return sum[:]
}

func (s *Server) oauth2Config(redirect string) *oauth2.Config {
	cfg := &oauth2.Config{
		ClientID:    s.cfg.HelloClientID,
		RedirectURL: redirect,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://wallet.hello.coop/authorize",
			TokenURL: "https://wallet.hello.coop/oauth/token",
		},
		Scopes: []string{oidc.ScopeOpenID, "name", "email"},
	}
	if sec := strings.TrimSpace(s.cfg.HelloClientSecret); sec != "" {
		cfg.ClientSecret = sec
	}
	return cfg
}

func (s *Server) handleHelloLogin(w http.ResponseWriter, r *http.Request) {
	if !s.helloEnabled() {
		http.Error(w, "Hellō login not configured", http.StatusServiceUnavailable)
		return
	}
	redirect := s.helloRedirectURIForRequest(r)
	if redirect == "" {
		http.Error(w, "PUBLIC_BASE_URL or HELLO_REDIRECT_URI required", http.StatusServiceUnavailable)
		return
	}

	state, err := randomURLString(24)
	if err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", err)
		return
	}
	nonce, err := randomURLString(24)
	if err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", err)
		return
	}
	verifier := oauth2.GenerateVerifier()

	oc := oauthCookie{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: verifier,
		RedirectURI:  redirect,
		ReturnTo:     sanitizeReturnTo(r.URL.Query().Get("return_to")),
		Exp:          time.Now().Add(oauthCookieTTL).Unix(),
	}
	// Prefer server-side state: some browsers drop Lax cookies across the Hellō round-trip.
	if raw, err := json.Marshal(oc); err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", err)
		return
	} else if err := s.store.PutIdempotency(r.Context(), oauthPendingKey(state), raw); err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", fmt.Errorf("store oauth state: %w", err))
		return
	}
	_ = s.setSignedCookie(w, r, oauthCookieName, oc, oauthCookieTTL)

	authURL := s.oauth2Config(redirect).AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oidc.Nonce(nonce),
		oauth2.SetAuthURLParam("response_mode", "query"),
	)
	// 200 + client navigate (not 302) so browsers reliably store Set-Cookie when present.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"/>
<link rel="icon" href="/favicon.png" type="image/png"/>
<meta http-equiv="refresh" content="0;url=%s"/>
<title>Continuing to Hellō…</title></head><body>
<p>Continuing to Hellō…</p>
<script>location.replace(%q)</script>
</body></html>`, htmlEscape(authURL), authURL)
}

func oauthPendingKey(state string) string {
	return "hello-oauth:" + state
}

func validOAuthState(state string) bool {
	if state == "" || len(state) > 200 {
		return false
	}
	for _, r := range state {
		switch {
		case r == '-' || r == '_':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func (s *Server) loadOAuthPending(r *http.Request, state string) (oauthCookie, error) {
	if !validOAuthState(state) {
		return oauthCookie{}, fmt.Errorf("missing state")
	}
	var oc oauthCookie
	if err := s.readSignedCookie(r, oauthCookieName, &oc); err == nil && oc.State == state {
		return oc, nil
	}
	raw, ok, err := s.store.GetIdempotency(r.Context(), oauthPendingKey(state))
	if err != nil {
		return oauthCookie{}, err
	}
	if !ok {
		return oauthCookie{}, fmt.Errorf("oauth state not found")
	}
	if err := json.Unmarshal(raw, &oc); err != nil {
		return oauthCookie{}, err
	}
	if oc.State != state {
		return oauthCookie{}, fmt.Errorf("oauth state mismatch")
	}
	return oc, nil
}

func (s *Server) handleHelloCallback(w http.ResponseWriter, r *http.Request) {
	if !s.helloEnabled() {
		http.Error(w, "Hellō login not configured", http.StatusServiceUnavailable)
		return
	}
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		log.Printf("hello auth denied: error=%q desc=%q", errMsg, r.URL.Query().Get("error_description"))
		http.Error(w, "Hellō login failed", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	oc, err := s.loadOAuthPending(r, state)
	if err != nil {
		log.Printf("hello oauth state: %v (host=%q)", err, r.Host)
		http.Error(w, "missing or expired login state — try Continue with Hellō again", http.StatusBadRequest)
		return
	}
	s.clearCookie(w, r, oauthCookieName)
	if time.Now().Unix() > oc.Exp {
		http.Error(w, "oauth state expired", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	redirect := oc.RedirectURI
	if redirect == "" {
		redirect = s.helloRedirectURIForRequest(r)
	}
	ctx := r.Context()
	provider, err := oidc.NewProvider(ctx, helloIssuer)
	if err != nil {
		writeAuthErr(w, http.StatusBadGateway, "login failed", fmt.Errorf("oidc provider: %w", err))
		return
	}
	oauthCfg := s.oauth2Config(redirect)
	tok, err := oauthCfg.Exchange(ctx, code, oauth2.VerifierOption(oc.CodeVerifier))
	if err != nil {
		writeAuthErr(w, http.StatusBadGateway, "login failed", fmt.Errorf("token exchange: %w", err))
		return
	}
	// Best-effort consume so the pending state cannot be replayed.
	_ = s.store.PutIdempotency(ctx, oauthPendingKey(state), []byte(`{"consumed":true}`))
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		writeAuthErr(w, http.StatusBadGateway, "login failed", fmt.Errorf("missing id_token"))
		return
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: s.cfg.HelloClientID})
	idToken, err := verifier.Verify(ctx, rawID)
	if err != nil {
		writeAuthErr(w, http.StatusBadGateway, "login failed", fmt.Errorf("id_token verify: %w", err))
		return
	}
	if idToken.Nonce != oc.Nonce {
		http.Error(w, "nonce mismatch", http.StatusBadRequest)
		return
	}
	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	_ = idToken.Claims(&claims)

	if !s.identityAllowed(idToken.Subject, claims.Email) {
		log.Printf("hello auth denied: not on allowlist email=%q sub=%q", claims.Email, idToken.Subject)
		http.Error(w, "This Hellō account is not authorized for tripmap.", http.StatusForbidden)
		return
	}

	sess := sessionCookie{
		Sub:   idToken.Subject,
		Email: claims.Email,
		Name:  claims.Name,
		Exp:   time.Now().Add(sessionCookieTTL).Unix(),
	}
	if err := s.setSignedCookie(w, r, sessionCookieName, sess, sessionCookieTTL); err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", err)
		return
	}
	http.Redirect(w, r, oc.ReturnTo, http.StatusFound)
}

func (s *Server) identityAllowed(sub, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	sub = strings.TrimSpace(sub)
	for _, e := range s.cfg.HelloAllowedEmails {
		if e != "" && e == email {
			return true
		}
	}
	for _, id := range s.cfg.HelloAllowedSubs {
		if id != "" && id == sub {
			return true
		}
	}
	return false
}

func (s *Server) sessionAllowed(sess sessionCookie) bool {
	return s.identityAllowed(sess.Sub, sess.Email)
}

func writeAuthErr(w http.ResponseWriter, status int, public string, err error) {
	if err != nil {
		log.Printf("hello auth: %v", err)
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
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.clearCookie(w, r, sessionCookieName)
	s.clearCookie(w, r, oauthCookieName)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) sessionFromRequest(r *http.Request) (sessionCookie, bool) {
	var sess sessionCookie
	if err := s.readSignedCookie(r, sessionCookieName, &sess); err != nil {
		return sessionCookie{}, false
	}
	if time.Now().Unix() > sess.Exp || sess.Sub == "" {
		return sessionCookie{}, false
	}
	if !s.sessionAllowed(sess) {
		return sessionCookie{}, false
	}
	return sess, true
}

func (s *Server) setSignedCookie(w http.ResponseWriter, r *http.Request, name string, v any, ttl time.Duration) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.sessionKey())
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	val := payload + "." + sig
	// Hellō SDK scopes the short-lived OIDC cookie to the auth endpoint; session stays "/".
	path := "/"
	if name == oauthCookieName {
		path = "/auth/hello"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    val,
		Path:     path,
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *Server) readSignedCookie(r *http.Request, name string, dest any) error {
	c, err := r.Cookie(name)
	if err != nil {
		return err
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		return fmt.Errorf("bad cookie format")
	}
	payload, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, s.sessionKey())
	_, _ = mac.Write([]byte(payload))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return fmt.Errorf("bad cookie signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dest)
}

func (s *Server) clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	path := "/"
	if name == oauthCookieName {
		path = "/auth/hello"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// cookieSecure is true for https PublicBaseURL (prod) so clients cannot weaken
// the flag via a spoofed X-Forwarded-Proto on a direct origin hit. Local http
// bases still honor TLS / forwarded proto for dev.
func (s *Server) cookieSecure(r *http.Request) bool {
	if strings.HasPrefix(strings.ToLower(s.cfg.PublicBaseURL), "https://") {
		return true
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func randomURLString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func sanitizeReturnTo(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") {
		return "/"
	}
	if strings.Contains(v, "://") {
		return "/"
	}
	for _, r := range v {
		switch {
		case r == '/' || r == '_' || r == '-' || r == '.' || r == '~':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		default:
			return "/"
		}
	}
	return v
}
