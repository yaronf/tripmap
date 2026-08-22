package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yaronf/tripmap/internal/store"
)

func TestSignedSessionCookieRoundTrip(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{
		AgentBearerToken: "secret",
		DescopeProjectID: "Ptest",
		SessionSecret:    "session-test-key",
		AllowedEmails:    []string{"a@b.c"},
		PublicBaseURL:    "https://tripmap.sheffer.org",
		MaxYAMLBytes:     1024,
		RouteMode:        "straight",
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
		"":                   "/",
		"/me/trips/holland/": "/me/trips/holland/",
		"//evil.com":         "/",
		"https://evil.com":   "/",
		"/auth/me":           "/auth/me",
		"relative":           "/",
		`/\evil.com`:         "/",
		"/%2f%2fevil.com":    "/",
		"/ok?x=1":            "/",
		"/a b":               "/",
	}
	for in, want := range cases {
		if got := sanitizeReturnTo(in); got != want {
			t.Fatalf("sanitizeReturnTo(%q)=%q want %q", in, got, want)
		}
	}
}

func TestAuthLoginRequiresConfig(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{AgentBearerToken: "secret", MaxYAMLBytes: 1024, RouteMode: "straight"}, mem)
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestAuthCallbackURIUsesRequestHostOnLoopback(t *testing.T) {
	srv := New(Config{
		AgentBearerToken: "secret",
		DescopeProjectID: "Ptest",
		SessionSecret:    "session-test-key",
		AllowedEmails:    []string{"a@b.c"},
		PublicBaseURL:    "http://localhost:8080",
		MaxYAMLBytes:     1024,
		RouteMode:        "straight",
	}, store.NewMem())
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	req.Host = "127.0.0.1:8080"
	got := srv.authCallbackURIForRequest(req)
	if got != "http://127.0.0.1:8080/auth/callback" {
		t.Fatalf("got %q", got)
	}
}

func TestRootShowsGoogleButton(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{
		AgentBearerToken: "secret",
		DescopeProjectID: "Ptest",
		SessionSecret:    "session-test-key",
		AllowedEmails:    []string{"a@b.c"},
		PublicBaseURL:    "https://tripmap.sheffer.org",
		MaxYAMLBytes:     1024,
		RouteMode:        "straight",
	}, mem)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Continue with Google") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestIdentityAllowedEmailOnly(t *testing.T) {
	srv := New(Config{
		AgentBearerToken: "secret",
		AllowedEmails:    []string{"yaronf@gmx.com"},
		AllowedSubs:      []string{"sub_ignored"},
		MaxYAMLBytes:     1024,
		RouteMode:        "straight",
	}, store.NewMem())
	if !srv.identityAllowed("YaronF@gmx.com") {
		t.Fatal("email allowlist should match case-insensitively")
	}
	if srv.identityAllowed("nope@example.com") {
		t.Fatal("expected deny")
	}
	if srv.identityAllowed("") {
		t.Fatal("empty email deny")
	}
}
