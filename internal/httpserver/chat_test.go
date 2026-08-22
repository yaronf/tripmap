package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatAuthGates(t *testing.T) {
	srv, _ := testServer(t)
	srv.cfg.DescopeProjectID = "app_test"
	srv.cfg.SessionSecret = "session-test-key"
	srv.cfg.AllowedEmails = []string{"a@b.c", "chat@b.c"}
	srv.cfg.ChatAllowedEmails = []string{"chat@b.c"}
	// Key set after New so chat handler stays nil → 503 (agent not wired).
	srv.cfg.OpenAIAPIKey = "sk-test"
	srv.cfg.OpenAIModel = "gpt-5-mini"

	createBody, _ := json.Marshal(map[string]string{"id": "chat-trip", "yaml": sampleYAML})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req.Header.Set("Idempotency-Key", "chat-c")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %s", rec.Body.String())
	}

	base := "/me/trips/chat-trip/"
	post := func(email string, withCookie bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, base+"api/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
		if withCookie {
			recSet := httptest.NewRecorder()
			sess := sessionCookie{Sub: "u1", Email: email, Name: "T", Exp: time.Now().Add(time.Hour).Unix()}
			if err := srv.setSignedCookie(recSet, req, sessionCookieName, sess, time.Hour); err != nil {
				t.Fatal(err)
			}
			for _, c := range recSet.Result().Cookies() {
				req.AddCookie(c)
			}
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	if rec := post("", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no session: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := post("a@b.c", true); rec.Code != http.StatusForbidden {
		t.Fatalf("no chat ACL: code=%d body=%s", rec.Code, rec.Body.String())
	}
	// OpenAI key set but agent not wired yet → 503.
	if rec := post("chat@b.c", true); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no agent: code=%d body=%s", rec.Code, rec.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	recSet := httptest.NewRecorder()
	sess := sessionCookie{Sub: "u1", Email: "chat@b.c", Name: "T", Exp: time.Now().Add(time.Hour).Unix()}
	if err := srv.setSignedCookie(recSet, meReq, sessionCookieName, sess, time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, c := range recSet.Result().Cookies() {
		meReq.AddCookie(c)
	}
	meRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("me code=%d", meRec.Code)
	}
	var me map[string]any
	if err := json.Unmarshal(meRec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me["chat_enabled"] != false {
		t.Fatalf("chat_enabled should be false until agent is wired, got %#v", me["chat_enabled"])
	}
}
