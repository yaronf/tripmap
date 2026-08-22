package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLegalPages(t *testing.T) {
	srv, _ := testServer(t)
	for _, path := range []string{"/privacy", "/terms"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", path, rec.Code)
		}
		body := rec.Body.String()
		if path == "/privacy" && !strings.Contains(body, "Privacy Policy") {
			t.Fatalf("%s: missing title", path)
		}
		if path == "/terms" && !strings.Contains(body, "Terms of Service") {
			t.Fatalf("%s: missing title", path)
		}
		if !strings.Contains(body, `href="/privacy"`) || !strings.Contains(body, `href="/terms"`) {
			t.Fatalf("%s: missing footer links", path)
		}
	}
}

func TestRootLinksLegalPages(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Tripmap</title>",
		"<h1>Tripmap</h1>",
		"Purpose of the application",
		`meta name="application-name" content="Tripmap"`,
		`property="og:site_name" content="Tripmap"`,
		`href="/privacy"`,
		`href="/terms"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("root missing %q", want)
		}
	}
}

func TestGoogleSiteVerificationMeta(t *testing.T) {
	srv := New(Config{
		AgentBearerToken:       "secret",
		PublicBaseURL:          "https://example.test",
		MaxYAMLBytes:           512 * 1024,
		GoogleSiteVerification: "abc123token",
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `name="google-site-verification" content="abc123token"`) {
		t.Fatalf("missing verification meta: %s", body)
	}
}

func TestRootShowsHelloWhenConfigured(t *testing.T) {
	srv := New(Config{
		AgentBearerToken:   "secret",
		PublicBaseURL:      "https://example.test",
		MaxYAMLBytes:         512 * 1024,
		HelloClientID:      "app_test",
		HelloSessionSecret: "test-secret",
		HelloAllowedEmails: []string{"a@example.com"},
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Continue with Hellō") {
		t.Fatalf("expected Hellō sign-in button")
	}
}
