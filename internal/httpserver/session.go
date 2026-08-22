package httpserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	loginCookieName   = "tripmap_login"
	sessionCookieName = "tripmap_session"
	loginCookieTTL    = 10 * time.Minute
	sessionCookieTTL  = 7 * 24 * time.Hour
)

type loginPendingCookie struct {
	ReturnTo string `json:"rt,omitempty"`
	Exp      int64  `json:"exp"`
}

type sessionCookie struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	Exp   int64  `json:"exp"`
}

func (s *Server) sessionKey() []byte {
	sum := sha256.Sum256([]byte(s.cfg.SessionSecret))
	return sum[:]
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
	path := "/"
	if name == loginCookieName {
		path = "/auth"
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
	if name == loginCookieName {
		path = "/auth"
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
