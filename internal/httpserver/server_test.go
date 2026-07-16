package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	srv := New(Config{AgentBearerToken: "test-token"})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %#v", body)
	}
}

func TestAgentRequiresBearer(t *testing.T) {
	srv := New(Config{AgentBearerToken: "secret"})
	req := httptest.NewRequest(http.MethodGet, "/api/agent/trips", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAgentWithBearerNotImplemented(t *testing.T) {
	srv := New(Config{AgentBearerToken: "secret"})
	req := httptest.NewRequest(http.MethodGet, "/api/agent/trips", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestOpenAPIPublic(t *testing.T) {
	srv := New(Config{AgentBearerToken: "secret", PublicBaseURL: "https://example.test"})
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "https://example.test") {
		t.Fatalf("missing base url in openapi")
	}
}

func TestLoadConfigFromJSONSecret(t *testing.T) {
	t.Setenv("AGENT_BEARER_TOKEN", "")
	t.Setenv("AGENT_BEARER_SECRET_JSON", `{"token":"from-json"}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentBearerToken != "from-json" {
		t.Fatalf("token = %q", cfg.AgentBearerToken)
	}
}

func TestLoadConfigRequiresToken(t *testing.T) {
	t.Setenv("AGENT_BEARER_TOKEN", "")
	t.Setenv("AGENT_BEARER_SECRET_JSON", "")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected error")
	}
}
