package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yaronf/tripmap/internal/store"
)

func TestSignedSessionCookieRoundTrip(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{
		AgentBearerToken:   "secret",
		HelloClientID:      "app_test",
		HelloSessionSecret: "session-test-key",
		HelloAllowedEmails: []string{"a@b.c"},
		PublicBaseURL:      "https://tripmap.sheffer.org",
		MaxYAMLBytes:       1024,
		RouteMode:          "straight",
	}, mem)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sess := sessionCookie{Sub: "user-1", Email: "a@b.c", Name: "A", Exp: time.Now().Add(time.Hour).Unix()}
	if err := srv.setSignedCookie(rec, req, sessionCookieName, sess, time.Hour); err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	req2 := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req2.AddCookie(cookies[0])
	got, ok := srv.sessionFromRequest(req2)
	if !ok {
		t.Fatal("expected session")
	}
	if got.Sub != "user-1" || got.Email != "a@b.c" {
		t.Fatalf("got %+v", got)
	}
}

func TestSanitizeReturnTo(t *testing.T) {
	cases := map[string]string{
		"":                 "/",
		"/me/trips/holland/": "/me/trips/holland/",
		"//evil.com":       "/",
		"https://evil.com": "/",
		"/auth/me":         "/auth/me",
		"relative":         "/",
		`/\evil.com`:       "/",
		"/%2f%2fevil.com":  "/",
		"/ok?x=1":          "/",
		"/a b":             "/",
	}
	for in, want := range cases {
		if got := sanitizeReturnTo(in); got != want {
			t.Fatalf("sanitizeReturnTo(%q)=%q want %q", in, got, want)
		}
	}
}

func TestHelloLoginRequiresConfig(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{AgentBearerToken: "secret", MaxYAMLBytes: 1024, RouteMode: "straight"}, mem)
	req := httptest.NewRequest(http.MethodGet, "/auth/hello/login", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestHelloLoginUsesRequestHostOnLoopback(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{
		AgentBearerToken:   "secret",
		HelloClientID:      "app_test",
		HelloSessionSecret: "session-test-key",
		HelloAllowedEmails: []string{"a@b.c"},
		HelloRedirectURI:   "http://localhost:8080/auth/hello/callback",
		PublicBaseURL:      "http://localhost:8080",
		MaxYAMLBytes:       1024,
		RouteMode:          "straight",
	}, mem)
	req := httptest.NewRequest(http.MethodGet, "/auth/hello/login", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if !stringsContains(rec.Body.String(), "redirect_uri=http%3A%2F%2F127.0.0.1%3A8080%2Fauth%2Fhello%2Fcallback") {
		t.Fatalf("body should use 127.0.0.1 redirect: %s", rec.Body.String()[:min(400, rec.Body.Len())])
	}
	if len(rec.Result().Cookies()) != 1 || rec.Result().Cookies()[0].Name != oauthCookieName {
		t.Fatalf("expected oauth cookie")
	}
}

func TestRootShowsHelloButton(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{
		AgentBearerToken:   "secret",
		HelloClientID:      "app_test",
		HelloSessionSecret: "session-test-key",
		HelloAllowedEmails: []string{"a@b.c"},
		PublicBaseURL:      "https://tripmap.sheffer.org",
		MaxYAMLBytes:       1024,
		RouteMode:          "straight",
	}, mem)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if !stringsContains(rec.Body.String(), "Continue with Hellō") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestIdentityAllowed(t *testing.T) {
	srv := New(Config{
		AgentBearerToken:   "secret",
		HelloAllowedEmails: []string{"yaronf@gmx.com"},
		HelloAllowedSubs:   []string{"sub_allowed"},
		MaxYAMLBytes:       1024,
		RouteMode:          "straight",
	}, store.NewMem())
	if !srv.identityAllowed("other", "YaronF@gmx.com") {
		t.Fatal("email allowlist should match case-insensitively")
	}
	if !srv.identityAllowed("sub_allowed", "nope@example.com") {
		t.Fatal("sub allowlist")
	}
	if srv.identityAllowed("nope", "nope@example.com") {
		t.Fatal("expected deny")
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})()))
}
