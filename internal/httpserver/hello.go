package httpserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
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
	redirect := s.helloRedirectURI()
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
		ReturnTo:     sanitizeReturnTo(r.URL.Query().Get("return_to")),
		Exp:          time.Now().Add(oauthCookieTTL).Unix(),
	}
	if err := s.setSignedCookie(w, r, oauthCookieName, oc, oauthCookieTTL); err != nil {
		writeAuthErr(w, http.StatusInternalServerError, "login failed", err)
		return
	}

	authURL := s.oauth2Config(redirect).AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oidc.Nonce(nonce),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
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

	var oc oauthCookie
	if err := s.readSignedCookie(r, oauthCookieName, &oc); err != nil {
		http.Error(w, "missing or invalid oauth state cookie", http.StatusBadRequest)
		return
	}
	s.clearCookie(w, r, oauthCookieName)
	if time.Now().Unix() > oc.Exp {
		http.Error(w, "oauth state expired", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != oc.State {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	redirect := s.helloRedirectURI()
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

func writeAuthErr(w http.ResponseWriter, status int, public string, err error) {
	if err != nil {
		log.Printf("hello auth: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": public})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionFromRequest(r)
	if !ok {
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
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    val,
		Path:     "/",
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
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
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
