package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yaronf/tripmap/internal/store"
)

func TestChatRequiresSignIn(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/me/trips/x/api/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want 401 body=%s", rec.Code, rec.Body.String())
	}
}

func TestChatForbiddenWithoutAllowlist(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{
		AgentBearerToken:   "secret",
		PublicBaseURL:      "https://example.test",
		MaxYAMLBytes:       512 * 1024,
		RouteMode:          "straight",
		HelloClientID:      "app_test",
		HelloSessionSecret: "session-test-key",
		HelloAllowedEmails: []string{"a@b.c"},
		OpenAIAPIKey:       "sk-test",
		ChatAllowedEmails:  []string{"other@x.y"},
	}, mem)

	createBody, _ := json.Marshal(map[string]string{"id": "chat-trip", "yaml": sampleYAML})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req.Header.Set("Idempotency-Key", "chat-c")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %s", rec.Body.String())
	}

	cookie := signedSessionCookie(t, srv, "user-1", "a@b.c")
	req = httptest.NewRequest(http.MethodPost, "/me/trips/chat-trip/api/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d want 403 body=%s", rec.Code, rec.Body.String())
	}
}

func TestChatUnavailableWithoutAPIKey(t *testing.T) {
	srv, _ := testServer(t)
	srv.cfg.HelloClientID = "app_test"
	srv.cfg.HelloSessionSecret = "session-test-key"
	srv.cfg.HelloAllowedEmails = []string{"a@b.c"}
	srv.cfg.ChatAllowedEmails = []string{"a@b.c"}
	// OpenAIAPIKey empty and chat handler nil → 503

	createBody, _ := json.Marshal(map[string]string{"id": "chat-nokey", "yaml": sampleYAML})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req.Header.Set("Idempotency-Key", "chat-nk")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %s", rec.Body.String())
	}

	cookie := signedSessionCookie(t, srv, "user-1", "a@b.c")
	req = httptest.NewRequest(http.MethodPost, "/me/trips/chat-nokey/api/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d want 503 body=%s", rec.Code, rec.Body.String())
	}
}

func signedSessionCookie(t *testing.T, srv *Server, sub, email string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sess := sessionCookie{Sub: sub, Email: email, Name: "Ada", Exp: time.Now().Add(time.Hour).Unix()}
	if err := srv.setSignedCookie(rec, req, sessionCookieName, sess, time.Hour); err != nil {
		t.Fatal(err)
	}
	return rec.Result().Cookies()[0]
}
