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
		"/t/holland/x/":    "/t/holland/x/",
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

func TestRootShowsHelloButton(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{
		AgentBearerToken:   "secret",
		HelloClientID:      "app_test",
		HelloSessionSecret: "session-test-key",
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
